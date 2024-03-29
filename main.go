package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	database "github.com/ad/go-githublistener/db"
	ghapi "github.com/ad/go-githublistener/ghapi"
	telegram "github.com/ad/go-githublistener/telegram"

	dlog "github.com/amoghe/distillog"
	sql "github.com/lazada/sqle"
	cron "github.com/robfig/cron/v3"
	tgbotapi "gopkg.in/telegram-bot-api.v4"
)

const version = "0.0.4"

var (
	err error

	bot    *tgbotapi.BotAPI
	db     *sql.DB
	client *ghapi.Client

	clientID     string
	clientSecret string

	httpPort        int
	httpRedirectURI string

	telegramToken         string
	telegramProxyHost     string
	telegramProxyPort     string
	telegramProxyUser     string
	telegramProxyPassword string
	telegramDebug         bool

	checkReposEvery   string
	checkCommitsEvery string
)

func main() {
	dlog.Infof("Started version %s", version)

	flag.StringVar(&clientID, "client_id", lookupEnvOrString("GO_GITHUB_LISTENER_CLIENT_ID", clientID), "github client id")
	flag.StringVar(&clientSecret, "client_secret", lookupEnvOrString("GO_GITHUB_LISTENER_CLIENT_SECRET", clientSecret), "github client secret")

	flag.IntVar(&httpPort, "http_port", lookupEnvOrInt("GO_GITHUB_LISTENER_PORT", 8080), "bot http port")
	flag.StringVar(&httpRedirectURI, "http_redirect_uri", lookupEnvOrString("GO_GITHUB_LISTENER_HTTP_REDIRECT_URI", "http://localhost:8080/oauth/redirect"), "http redirect uri")

	flag.StringVar(&telegramToken, "telegram_token", lookupEnvOrString("GO_GITHUB_LISTENER_TELEGRAM_TOKEN", telegramToken), "telegramToken")
	flag.StringVar(&telegramProxyHost, "telegram_proxy_host", lookupEnvOrString("GO_GITHUB_LISTENER_TELEGRAM_PROXY_HOST", telegramProxyHost), "telegramProxyHost")
	flag.StringVar(&telegramProxyPort, "telegram_proxy_port", lookupEnvOrString("GO_GITHUB_LISTENER_TELEGRAM_PROXY_PORT", telegramProxyPort), "telegramProxyPort")
	flag.StringVar(&telegramProxyUser, "telegram_proxy_user", lookupEnvOrString("GO_GITHUB_LISTENER_TELEGRAM_PROXY_USER", telegramProxyUser), "telegramProxyUser")
	flag.StringVar(&telegramProxyPassword, "telegram_proxy_password", lookupEnvOrString("GO_GITHUB_LISTENER_TELEGRAM_PROXY_PASSWORD", telegramProxyPassword), "telegramProxyPassword")
	flag.BoolVar(&telegramDebug, "telegram_debug", lookupEnvOrBool("GO_GITHUB_LISTENER_TELEGRAM_DEBUG", telegramDebug), "telegramDebug")

	flag.StringVar(&checkReposEvery, "check_repos_every", lookupEnvOrString("GO_GITHUB_LISTENER_CHECK_REPOS_EVERY", "*/15 * * * *"), "run cron job for check repos every")
	flag.StringVar(&checkCommitsEvery, "check_commits_every", lookupEnvOrString("GO_GITHUB_LISTENER_CHECK_COMMITS_EVERY", "* * * * *"), "run cron job for check commits every")

	flag.Parse()
	log.SetFlags(0)

	client = ghapi.NewClient(clientID, clientSecret)

	// Init DB
	db, err = database.InitDB()
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
		return
	}
	defer func() { _ = db.Close() }()

	// Init telegram
	bot, err = telegram.InitTelegram(telegramToken, telegramProxyHost, telegramProxyPort, telegramProxyUser, telegramProxyPassword, telegramDebug)
	if err != nil {
		log.Fatalf("fail on telegram login: %v", err)
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		log.Fatalf("[INIT] [Failed to init Telegram updates chan: %v]", err)
	}

	go processTelegramMessages(updates)

	http.HandleFunc("/oauth/redirect", func(w http.ResponseWriter, r *http.Request) {
		// dlog.Debugf("parse query: %#v", r)

		err12 := r.ParseForm()
		if err12 != nil {
			dlog.Errorf("could not parse query: %v", err12)
			w.WriteHeader(http.StatusBadRequest)
		}
		code := r.FormValue("code")

		token, err13 := client.GetGithubUserAccessToken(code)
		if err13 != nil {
			dlog.Errorln(err13)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		//w.Header().Set("Location", "/user?access_token="+t.AccessToken)
		w.Header().Set("Location", "https://t.me/"+bot.Self.UserName+"?start="+token)
		w.WriteHeader(http.StatusFound)
	})

	dlog.Debugf("Listening on port %d", httpPort)

	cron := cron.New()
	_, err = cron.AddFunc(checkCommitsEvery, func() {
		// ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cronDisableTimeout)*time.Second)
		// defer cancel()

		dlog.Debugln("started check commits cron job")

		if usersRepos, err14 := database.GetUserRepos(db); err14 != nil {
			dlog.Errorln(err14)
		} else if len(usersRepos) > 0 {
			for _, item := range usersRepos {
				telegramUserID, err15 := strconv.ParseInt(item.TelegramUserID, 10, 64)
				if err15 != nil {
					dlog.Errorln(err15)
					continue
				}
				// dlog.Infof("ITEM in:  %#v %s", item, item.UpdatedAt.String())
				go func(item *database.UsersReposResult) {
					commits, err16 := client.GetGithubUserRepoCommits(item)
					if err16 != nil {
						dlog.Errorln(err16)
						if err16.Error() == "repo not found" {
							if errDeleteRepo := database.DeleteRepoUserLinkDB(db, &database.GithubUser{ID: item.UserID}, &database.GithubRepo{ID: item.RepoID}); err == nil {
								dlog.Infof("%s %s %d", item.RepoName, "removed for", item.UserID)
								msg := tgbotapi.NewMessage(telegramUserID, "repo "+item.RepoName+" not found, removed")
								_, err9 := bot.Send(msg)
								if err9 != nil {
									dlog.Errorln(err9)
								}
							} else {
								dlog.Errorln(errDeleteRepo)
							}
						}
						return
					}

					if len(commits) > 0 {
						// dlog.Debugf("ITEM in:  %#v %s", item, item.UpdatedAt.String())
						// dlog.Infof("%#v", commits)

						for _, commit := range commits {
							if commit.Commit.Author.Date.After(item.UpdatedAt) {
								item.UpdatedAt = commit.Commit.Author.Date
							}

							if commit.Commit.Committer.Date.After(item.UpdatedAt) {
								item.UpdatedAt = commit.Commit.Committer.Date
							}

							msg := tgbotapi.NewMessage(telegramUserID, "")
							msg.ParseMode = "Markdown"
							msg.DisableWebPagePreview = true
							msg.Text += "[" + item.RepoName + "](https://github.com/" + item.RepoName + ") was updated by [" + commit.Commit.Author.Name + "](https://github.com/" + commit.Commit.Author.Name + ") with new commit([" + commit.SHA + "](" + commit.HTMLUrl + ")):\n" + commit.Commit.Message
							_, err17 := bot.Send(msg)
							if err17 != nil {
								dlog.Errorln(err17)
							}
						}

						// dlog.Debugf("ITEM out: %#v %s", item, item.UpdatedAt.String())
						err18 := database.UpdateUserRepoLink(db, item)
						if err18 != nil {
							dlog.Errorln(err18)
						}
					}
				}(item)
			}
		}
	})
	if err != nil {
		dlog.Errorf("wrong cronjob params: %s", err)
	}
	_, err2 := cron.AddFunc(checkReposEvery, func() {
		dlog.Debugln("started check repos cron job")

		if users, err14 := database.GetUsers(db); err14 != nil {
			dlog.Errorln(err14)
		} else if len(users) > 0 {
			for _, ghuser := range users {
				if repos, err6 := client.GetGithubUserRepos(ghuser.Token, ghuser.UserName); err6 == nil {
					for _, repo := range repos {

						ghrepo := &database.GithubRepo{
							Name:     repo.Name,
							RepoName: repo.FullName,
						}

						if dbrepo, err7 := database.AddRepoIfNotExist(db, ghrepo); err7 != nil && err7.Error() != database.AlreadyExists {
							dlog.Errorln(err7)
						} else if err8 := database.AddRepoLinkIfNotExist(db, ghuser, dbrepo, repo.UpdatedAt); err8 != nil && err8.Error() != database.AlreadyExists {
							dlog.Errorln(err8)
						}
					}
				} else {
					dlog.Errorln(err6)
				}
			}
		}
	})
	if err2 != nil {
		dlog.Errorf("wrong cronjob params: %s", err2)
	}
	cron.Start()
	defer cron.Stop()

	log.Fatal(http.ListenAndServe("0.0.0.0:"+strconv.Itoa(httpPort), nil))
}

func processTelegramMessages(updates tgbotapi.UpdatesChannel) {
	for update := range updates {
		if update.Message == nil { // ignore any non-Message Updates
			continue
		}

		dlog.Infof("%s [%d] %s", update.Message.From.UserName, update.Message.From.ID, update.Message.Text)

		message := database.TelegramMessage{
			UserID:   update.Message.From.ID,
			UserName: update.Message.From.UserName,
			Message:  update.Message.Text,
			Date:     time.Unix(int64(update.Message.Date), 0),
		}

		err2 := database.StoreTelegramMessage(db, message)
		if err2 != nil {
			dlog.Errorf("%s", err2)
		}

		if update.Message.IsCommand() {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")
			switch update.Message.Command() {
			case "start", "startgroup", "repos":
				ghuser := &database.GithubUser{
					TelegramUserID: strconv.Itoa(update.Message.From.ID),
				}

				if update.Message.Command() != "repos" && update.Message.CommandArguments() != "" {
					if user, err3 := client.GetGithubUser(update.Message.CommandArguments()); err3 == nil {
						if user.Name != "" {
							msg.Text = "Hi, " + user.Name
						} else {
							msg.Text = "Hi, " + user.UserName
						}

						ghuser.Name = user.Name
						ghuser.UserName = user.UserName
						ghuser.Token = update.Message.CommandArguments()

						dbuser, err4 := database.AddUserIfNotExist(db, ghuser)
						if err4 != nil && err4.Error() != database.AlreadyExists {
							msg.Text += "\nError on save your token, try /start again\n" + err4.Error()
							_, err5 := bot.Send(msg)
							if err5 != nil {
								dlog.Errorln(err5)
							}
							continue
						}
						ghuser.ID = dbuser.ID
					}
				} else {
					if user, err20 := database.GetGithubUserFromDB(db, ghuser.TelegramUserID); err20 == nil {
						ghuser.ID = user.ID
						ghuser.Name = user.Name
						ghuser.UserName = user.UserName
						ghuser.Token = user.Token
					}
				}

				if ghuser.ID != 0 {
					if repos, err6 := client.GetGithubUserRepos(ghuser.Token, ghuser.UserName); err6 == nil {
						msg2 := tgbotapi.NewMessage(update.Message.Chat.ID, "You are watching:\n")
						for _, repo := range repos {
							msg2.Text += "[" + repo.FullName + "](https://github.com/" + repo.FullName + ") updated at:" + repo.UpdatedAt.Format("2006-01-02 15:04:05") + "\n"

							ghrepo := &database.GithubRepo{
								Name:     repo.Name,
								RepoName: repo.FullName,
							}

							if dbrepo, err7 := database.AddRepoIfNotExist(db, ghrepo); err7 != nil && err7.Error() != database.AlreadyExists {
								dlog.Errorln(err7)
							} else if err8 := database.AddRepoLinkIfNotExist(db, ghuser, dbrepo, repo.UpdatedAt); err8 != nil && err8.Error() != database.AlreadyExists {
								dlog.Errorln(err8)
							}
						}
						msg2.ParseMode = "Markdown"
						msg2.DisableWebPagePreview = true
						_, err9 := bot.Send(msg2)
						if err9 != nil {
							dlog.Errorln(err9)
						}

						continue
					} else {
						dlog.Errorln(err6)
					}
				}

				text := `[Click here to authorize bot in github](https://github.com/login/oauth/authorize?client_id=` + clientID + `&redirect_uri=` + httpRedirectURI + `), and then press START again`
				msg.ParseMode = "Markdown"
				msg.Text = text
				msg.DisableWebPagePreview = true
			case "me":
				if user, err10 := database.GetGithubUserFromDB(db, strconv.Itoa(update.Message.From.ID)); err10 == nil {
					if user.Name != "" {
						msg.Text = "Hi, " + user.Name
					} else {
						msg.Text = "Hi, " + user.UserName
					}
				} else {
					msg.Text = "type /start\n"
					msg.Text += err10.Error()
				}
			case "delete":
				if checkRepoName(update.Message.CommandArguments()) {
					if ghuser, err10 := database.GetGithubUserFromDB(db, strconv.Itoa(update.Message.From.ID)); err10 == nil {
						if ghrepo, errGetRepo := database.GetGithubRepoByNameFromDB(db, update.Message.CommandArguments()); errGetRepo == nil {
							if errDeleteRepo := database.DeleteRepoUserLinkDB(db, ghuser, ghrepo); err == nil {
								dlog.Infof("%s %s %s", ghuser.Name, "removed", ghrepo.RepoName)
								msg.Text = ghrepo.RepoName + " removed, uncheck Watching in Github interface"
							} else {
								msg.Text += errDeleteRepo.Error()
							}
						} else {
							msg.Text = errGetRepo.Error()
						}
					} else {
						msg.Text = "type /start\n"
						msg.Text += err10.Error()
					}
				} else {
					msg.Text = "wrong repo format, try username/reponame instead"
				}
			case "add":
				if checkRepoName(update.Message.CommandArguments()) {
					if ghuser, err10 := database.GetGithubUserFromDB(db, strconv.Itoa(update.Message.From.ID)); err10 == nil {
						if ghrepo, errCheckRepo := database.GetGithubRepoByNameFromDB(db, update.Message.CommandArguments()); errCheckRepo == nil {
							if err8 := database.AddRepoLinkIfNotExist(db, ghuser, ghrepo, time.Now()); err8 != nil && err8.Error() != database.AlreadyExists {
								dlog.Errorln(err8)
							} else {
								msg.Text = ghrepo.RepoName + " added"
							}
						} else {
							if repo, errGetRepo := client.GetGithubRepo(ghuser.Token, update.Message.CommandArguments()); errGetRepo == nil {
								ghrepo := &database.GithubRepo{
									Name:     repo.Name,
									RepoName: repo.FullName,
								}
								if dbrepo, err7 := database.AddRepoIfNotExist(db, ghrepo); err7 != nil && err7.Error() != database.AlreadyExists {
									dlog.Errorln(err7)
								} else if err8 := database.AddRepoLinkIfNotExist(db, ghuser, dbrepo, repo.UpdatedAt); err8 != nil && err8.Error() != database.AlreadyExists {
									dlog.Errorln(err8)
								} else {
									msg.Text = ghrepo.RepoName + " added"
								}
							} else {
								msg.Text = ghrepo.RepoName + " not found"
							}
						}
					} else {
						msg.Text = "type /start\n"
						msg.Text += err10.Error()
					}
				} else {
					msg.Text = "wrong repo format, try username/reponame instead"
				}
			case "help":
				msg.Text = "type /start"
			default:
				msg.Text = "I don't know that command"
			}
			msg.ReplyToMessageID = update.Message.MessageID
			_, err11 := bot.Send(msg)
			if err11 != nil {
				dlog.Errorln(err11)
			}
		}
	}
}

func checkRepoName(s string) bool {
	re := regexp.MustCompile(`^([\w,\-,\_]+)\/([\w,\-,\_]+)$`)
	return re.Match([]byte(s))
}

func lookupEnvOrString(key string, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

func lookupEnvOrInt(key string, defaultVal int) int {
	if val, ok := os.LookupEnv(key); ok {
		if x, err := strconv.Atoi(val); err == nil {
			return x
		}
	}
	return defaultVal
}

func lookupEnvOrBool(key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		if val == "true" {
			return true
		}
		if val == "false" {
			return false
		}
	}
	return defaultVal
}
