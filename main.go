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

	tgbotapi "gopkg.in/telegram-bot-api.v4"

	"github.com/ad/go-githublistener/telegram"
)

const version = "0.0.1"

var (
	bot *tgbotapi.BotAPI
	err error

	httpClient = http.Client{
		Timeout: time.Duration(5 * time.Second),
	}

	clientID     string
	clientSecret string
	httpPort     int

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

	flag.StringVar(&telegramToken, "telegramToken", lookupEnvOrString("GO_GITHUB_LISTENER_TELEGRAM_TOKEN", telegramToken), "telegramToken")
	flag.StringVar(&telegramProxyHost, "telegramProxyHost", lookupEnvOrString("GO_GITHUB_LISTENER_TELEGRAM_PROXY_HOST", telegramProxyHost), "telegramProxyHost")
	flag.StringVar(&telegramProxyPort, "telegramProxyPort", lookupEnvOrString("GO_GITHUB_LISTENER_TELEGRAM_PROXY_PORT", telegramProxyPort), "telegramProxyPort")
	flag.StringVar(&telegramProxyUser, "telegramProxyUser", lookupEnvOrString("GO_GITHUB_LISTENER_TELEGRAM_PROXY_USER", telegramProxyUser), "telegramProxyUser")
	flag.StringVar(&telegramProxyPassword, "telegramProxyPassword", lookupEnvOrString("GO_GITHUB_LISTENER_TELEGRAM_PROXY_PASSWORD", telegramProxyPassword), "telegramProxyPassword")
	flag.BoolVar(&telegramDebug, "telegramDebug", lookupEnvOrBool("GO_GITHUB_LISTENER_TELEGRAM_DEBUG", telegramDebug), "telegramDebug")

	flag.Parse()
	log.SetFlags(0)

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

			msg := tgbotapi.NewMessage(update.Message.Chat.ID, update.Message.Text)
			msg.ReplyToMessageID = update.Message.MessageID

			bot.Send(msg)
		}
	}()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		indexHTML := `<!DOCTYPE html>
<html>
	<head>
        <meta charset="utf-8" />
        <meta http-equiv="X-UA-Compatible" content="IE=edge">
        <title>Github auth</title>
        <meta name="viewport" content="width=device-width, initial-scale=1">
	</head>
	<body>
        <a href="https://github.com/login/oauth/authorize?client_id=` + clientID + `&redirect_uri=http://localhost:8080/oauth/redirect&state=somerandomstring">Login with github</a>
	</body>
</html>`

		w.Write([]byte(indexHTML))
	})

	http.HandleFunc("/oauth/redirect", func(w http.ResponseWriter, r *http.Request) {
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

		w.Header().Set("Location", "/user?access_token="+t.AccessToken)
		w.WriteHeader(http.StatusFound)
	})

	http.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			fmt.Fprintf(os.Stdout, "could not parse query: %v", err)
			w.WriteHeader(http.StatusBadRequest)
		}
		code := r.FormValue("access_token")

		body, err := makeRequest("https://api.github.com/user", code)
		if err != nil {
			fmt.Fprintf(os.Stdout, "could not parse JSON response: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		var user UserResponse
		if err := json.Unmarshal(body, &user); err != nil {
			fmt.Fprintf(os.Stdout, "could not parse JSON response: %v", err)
			w.WriteHeader(http.StatusBadRequest)
		}

		w.Write([]byte(fmt.Sprintf("%#v", user)))
	})

	http.HandleFunc("/repos", func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			fmt.Fprintf(os.Stdout, "could not parse query: %v", err)
			w.WriteHeader(http.StatusBadRequest)
		}
		code := r.FormValue("access_token")
		username := r.FormValue("username")

		body, err := makeRequest("https://api.github.com/users/"+username+"/subscriptions", code)
		if err != nil {
			fmt.Fprintf(os.Stdout, "could not parse JSON response: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		var repos []Repo
		if err := json.Unmarshal(body, &repos); err != nil {
			fmt.Fprintf(os.Stdout, "could not parse JSON response: %v", err)
			w.WriteHeader(http.StatusBadRequest)
		}

		w.Write([]byte(fmt.Sprintf("%#v", repos)))
	})

	http.HandleFunc("/commits", func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			fmt.Fprintf(os.Stdout, "could not parse query: %v", err)
			w.WriteHeader(http.StatusBadRequest)
		}
		code := r.FormValue("access_token")
		repo := r.FormValue("repo")
		since := r.FormValue("since")

		body, err := makeRequest("https://api.github.com/repos/"+repo+"/commits?since="+since, code)
		if err != nil {
			fmt.Fprintf(os.Stdout, "could not parse JSON response: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		var commits []CommitItem
		if err := json.Unmarshal(body, &commits); err != nil {
			fmt.Fprintf(os.Stdout, "could not parse JSON response: %v", err)
			w.WriteHeader(http.StatusBadRequest)
		}

		w.Write([]byte(fmt.Sprintf("%#v", commits)))
	})

	log.Printf("Listening on port %d", httpPort)

	log.Fatal(http.ListenAndServe("0.0.0.0:"+strconv.Itoa(httpPort), nil))
}

type OAuthAccessResponse struct {
	AccessToken string `json:"access_token"`
}

type UserResponse struct {
	Name     string `json:"name"`
	UserName string `json:"login"`
}

type Repo struct {
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	PushedAt string `json:"pushed_at"`
}

type CommitItem struct {
	Commit Commit `json:"commit"`
	URL    string `json:"url"`
}

type Commit struct {
	Author  Author `json:"author"`
	Message string `json:"message"`
}

type Author struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Date  string `json:"date"`
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