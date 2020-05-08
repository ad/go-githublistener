package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

const clientID = ""
const clientSecret = ""

var (
	client = http.Client{
		Timeout: time.Duration(5 * time.Second),
	}
)

func main() {
	fs := http.FileServer(http.Dir("public"))
	http.Handle("/", fs)

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

		res, err := client.Do(req)
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

	http.ListenAndServe(":8080", nil)
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

	resp, err := client.Do(request)
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
