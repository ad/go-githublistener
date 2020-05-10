package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	database "github.com/ad/go-githublistener/db"
	telegram "github.com/ad/go-githublistener/telegram"

	sql "github.com/lazada/sqle"
	tgbotapi "gopkg.in/telegram-bot-api.v4"
)

const version = "0.0.3"

var (
	err error

	bot *tgbotapi.BotAPI
	db  *sql.DB

	httpClient = http.Client{
		Timeout: time.Duration(5 * time.Second),
	}

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
)

func main() {
	log.Printf("Started version %s", version)

	flag.StringVar(&clientID, "client_id", lookupEnvOrString("GO_GITHUB_LISTENER_CLIENT_ID", clientID), "github client id")
	flag.StringVar(&clientSecret, "client_secret", lookupEnvOrString("GO_GITHUB_LISTENER_CLIENT_SECRET", clientSecret), "github client secret")

	flag.IntVar(&httpPort, "http_port", lookupEnvOrInt("GO_GITHUB_LISTENER_PORT", 8080), "bot http port")
	flag.StringVar(&httpRedirectURI, "http_redirect_uri", lookupEnvOrString("GO_GITHUB_LISTENER_HTTP_REDIRECT_URI", "http://localhost:8080/oauth/redirect"), "http redirect uri")

	flag.StringVar(&telegramToken, "telegramToken", lookupEnvOrString("GO_GITHUB_LISTENER_TELEGRAM_TOKEN", telegramToken), "telegramToken")
	flag.StringVar(&telegramProxyHost, "telegramProxyHost", lookupEnvOrString("GO_GITHUB_LISTENER_TELEGRAM_PROXY_HOST", telegramProxyHost), "telegramProxyHost")
	flag.StringVar(&telegramProxyPort, "telegramProxyPort", lookupEnvOrString("GO_GITHUB_LISTENER_TELEGRAM_PROXY_PORT", telegramProxyPort), "telegramProxyPort")
	flag.StringVar(&telegramProxyUser, "telegramProxyUser", lookupEnvOrString("GO_GITHUB_LISTENER_TELEGRAM_PROXY_USER", telegramProxyUser), "telegramProxyUser")
	flag.StringVar(&telegramProxyPassword, "telegramProxyPassword", lookupEnvOrString("GO_GITHUB_LISTENER_TELEGRAM_PROXY_PASSWORD", telegramProxyPassword), "telegramProxyPassword")
	flag.BoolVar(&telegramDebug, "telegramDebug", lookupEnvOrBool("GO_GITHUB_LISTENER_TELEGRAM_DEBUG", telegramDebug), "telegramDebug")

	flag.Parse()
	log.SetFlags(0)

	// Init DB
	db, err = database.InitDB()
	if err != nil {
		log.Printf("Failed to open database: %#+v\n", err)
		return
	}
	defer db.Close()

	// Init telegram
	bot, err = telegram.InitTelegram(telegramToken, telegramProxyHost, telegramProxyPort, telegramProxyUser, telegramProxyPassword, telegramDebug)
	if err != nil {
		log.Fatal("fail on telegram login:", err)
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		log.Fatalf("[INIT] [Failed to init Telegram updates chan: %v]", err)
	}

	go func() {
		for update := range updates {
			if update.Message == nil { // ignore any non-Message Updates
				continue
			}

			log.Printf("%s [%d] %s", update.Message.From.UserName, update.Message.From.ID, update.Message.Text)

			message := database.TelegramMessage{
				UserID:   update.Message.From.ID,
				UserName: update.Message.From.UserName,
				Message:  update.Message.Text,
				Date:     time.Unix(int64(update.Message.Date), 0),
			}

			err := database.StoreTelegramMessage(db, message)
			if err != nil {
				log.Println(err)
			}

			if update.Message.IsCommand() {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")
				switch update.Message.Command() {
				case "start", "startgroup":
					if update.Message.CommandArguments() != "" {
						if user, err := getGithubUser(update.Message.CommandArguments()); err == nil {
							msg.Text = "Hi, " + user.Name

							ghuser := &database.GithubUser{
								Name:           user.Name,
								UserName:       user.UserName,
								Token:          update.Message.CommandArguments(),
								TelegramUserID: strconv.Itoa(update.Message.From.ID),
							}

							dbuser, err := database.AddUserIfNotExist(db, ghuser)

							if err != nil && err.Error() != "already exists" {
								msg.Text += "\nError on save your token, try /start again\n" + err.Error()
								bot.Send(msg)

								return
							}

							if repos, err := getGithubUserRepos(update.Message.CommandArguments(), user.UserName); err == nil {
								msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")
								for _, repo := range repos {
									msg.Text += repo.Name + " - " + repo.FullName + " / " + repo.PushedAt.Format("2006-01-02 15:04:05") + "\n"

									ghrepo := &database.GithubRepo{
										Name:     repo.Name,
										RepoName: repo.FullName,
									}

									if dbrepo, err := database.AddRepoIfNotExist(db, ghrepo); err != nil && err.Error() != "already exists" {
										//msg.Text += "\nError on save your repo, try again\n" + err.Error()
									} else {
										if err := database.AddRepoLinkIfNotExist(db, dbuser, dbrepo, repo.PushedAt); err != nil && err.Error() != "already exists" {
											//msg.Text += "\nError on save your repo-to-user link, try again\n" + err.Error()
										} else {

										}
									}
								}

								bot.Send(msg)

								return
							}
							return
						}
					}

					text := `[Click here to authorize bot in github](https://github.com/login/oauth/authorize?client_id=` + clientID + `&redirect_uri=` + httpRedirectURI + `), and then press START again`
					msg.ParseMode = "Markdown"
					msg.Text = text
					msg.DisableWebPagePreview = true
				case "me":
					if user, err := getGithubUserFromDB(update.Message.From.ID); err == nil {
						msg.Text = "Hi, " + user.Name
					} else {
						msg.Text = "type /start\n"
						msg.Text += err.Error()
					}
				case "repos":
					if user, err := getGithubUserFromDB(update.Message.From.ID); err == nil {
						if repos, err := getGithubUserRepos(user.Token, user.UserName); err == nil {
							msg.Text += "Your repos list:\n"

							ghuser := &database.GithubUser{
								ID:             user.ID,
								Name:           user.Name,
								UserName:       user.UserName,
								Token:          update.Message.CommandArguments(),
								TelegramUserID: strconv.Itoa(update.Message.From.ID),
							}

							for _, repo := range repos {
								msg.Text += repo.Name + " - " + repo.FullName + " / " + repo.PushedAt.Format("2006-01-02 15:04:05") + "\n"

								ghrepo := &database.GithubRepo{
									Name:     repo.Name,
									RepoName: repo.FullName,
								}

								if dbrepo, err := database.AddRepoIfNotExist(db, ghrepo); err != nil && err.Error() != "already exists" {
									//msg.Text += "\nError on save your repo, try again\n" + err.Error()
								} else {
									if err := database.AddRepoLinkIfNotExist(db, ghuser, dbrepo, repo.PushedAt); err != nil && err.Error() != "already exists" {
										//msg.Text += "\nError on save your repo-to-user link, try again\n" + err.Error()
									} else {

									}
								}
							}
						} else {
							msg.Text += "Empty repos list\n"
							msg.Text += err.Error()
						}
					} else {
						msg.Text = "type /start\n"
						msg.Text += err.Error()
					}
				case "help":
					msg.Text = "type /me, /repos, /commits"
				default:
					msg.Text = "I don't know that command"
				}
				msg.ReplyToMessageID = update.Message.MessageID
				bot.Send(msg)
			}
		}
	}()

	http.HandleFunc("/oauth/redirect", func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("parse query: %#v", r)

		err := r.ParseForm()
		if err != nil {
			fmt.Fprintf(os.Stdout, "could not parse query: %v", err)
			w.WriteHeader(http.StatusBadRequest)
		}
		code := r.FormValue("code")

		reqURL := fmt.Sprintf("https://github.com/login/oauth/access_token?client_id=%s&client_secret=%s&code=%s", clientID, clientSecret, code)
		req, err := http.NewRequest(http.MethodPost, reqURL, nil)
		if err != nil {
			fmt.Fprintf(os.Stdout, "could not create HTTP request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
		}
		req.Header.Set("accept", "application/json")

		res, err := httpClient.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stdout, "could not send HTTP request: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		defer res.Body.Close()

		var t OAuthAccessResponse
		if err := json.NewDecoder(res.Body).Decode(&t); err != nil {
			fmt.Fprintf(os.Stdout, "could not parse JSON response: %v", err)
			w.WriteHeader(http.StatusBadRequest)
		}

		//w.Header().Set("Location", "/user?access_token="+t.AccessToken)
		w.Header().Set("Location", "https://t.me/"+bot.Self.UserName+"?start="+t.AccessToken)
		w.WriteHeader(http.StatusFound)
	})

	// http.HandleFunc("/commits", func(w http.ResponseWriter, r *http.Request) {
	// 	err := r.ParseForm()
	// 	if err != nil {
	// 		fmt.Fprintf(os.Stdout, "could not parse query: %v", err)
	// 		w.WriteHeader(http.StatusBadRequest)
	// 	}
	// 	code := r.FormValue("access_token")
	// 	repo := r.FormValue("repo")
	// 	since := r.FormValue("since")

	// 	body, err := makeRequest("https://api.github.com/repos/"+repo+"/commits?since="+since, code)
	// 	if err != nil {
	// 		fmt.Fprintf(os.Stdout, "could not parse JSON response: %v", err)
	// 		w.WriteHeader(http.StatusInternalServerError)
	// 		return
	// 	}

	// 	var commits []CommitItem
	// 	if err := json.Unmarshal(body, &commits); err != nil {
	// 		fmt.Fprintf(os.Stdout, "could not parse JSON response: %v", err)
	// 		w.WriteHeader(http.StatusBadRequest)
	// 	}

	// 	w.Write([]byte(fmt.Sprintf("%#v", commits)))
	// })

	log.Printf("Listening on port %d", httpPort)

	log.Fatal(http.ListenAndServe("0.0.0.0:"+strconv.Itoa(httpPort), nil))
}

// OAuthAccessResponse ...
type OAuthAccessResponse struct {
	AccessToken string `json:"access_token"`
}

// UserResponse ...
type UserResponse struct {
	Name     string `json:"name"`
	UserName string `json:"login"`
}

// Repo ...
type Repo struct {
	Name     string    `json:"name"`
	FullName string    `json:"full_name"`
	PushedAt time.Time `json:"pushed_at"`
}

// CommitItem ...
type CommitItem struct {
	Commit Commit `json:"commit"`
	URL    string `json:"url"`
}

// Commit ...
type Commit struct {
	Author  Author `json:"author"`
	Message string `json:"message"`
}

// Author ...
type Author struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Date  string `json:"date"`
}

func getGithubUser(code string) (*UserResponse, error) {
	body, err := makeRequest("https://api.github.com/user", code)
	if err != nil {
		return nil, err
	}

	var user UserResponse
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, err
	}

	return &user, nil
}

func getGithubUserFromDB(id int) (*database.GithubUser, error) {
	var returnModel database.GithubUser
	sql := fmt.Sprintf("SELECT * FROM github_users WHERE telegram_user_id=%s;", strconv.Itoa(id))

	result, err := database.QuerySQLObject(db, sql, returnModel)
	if err != nil {
		return nil, err
	}

	if returnModel, ok := result.Interface().(*database.GithubUser); ok && returnModel.UserName != "" {
		return returnModel, nil
	}

	return nil, fmt.Errorf("User not found")
}

func getGithubUserRepos(code, username string) ([]*Repo, error) {
	body, err := makeRequest("https://api.github.com/users/"+username+"/subscriptions", code)
	if err != nil {
		return nil, err
	}

	var repos []*Repo
	if err := json.Unmarshal(body, &repos); err != nil {
		return nil, fmt.Errorf("%s\n%s\n%s\n%s", err, string(body), "https://api.github.com/users/"+username+"/subscriptions", code)
	}

	return repos, nil
}

func makeRequest(url, token string) ([]byte, error) {
	request, err := http.NewRequest("GET", url, nil)
	request.Header.Set("Authorization", "token "+token)

	resp, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
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
