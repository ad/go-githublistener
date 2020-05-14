package ghapi

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	database "github.com/ad/go-githublistener/db"
)

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
	Name      string    `json:"name"`
	FullName  string    `json:"full_name"`
	UpdatedAt time.Time `json:"updated_at"`
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
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Date  time.Time `json:"date"`
}

// Client ...
type Client struct {
	HTTPClient   http.Client
	clientID     string
	clientSecret string
}

// NewClient ...
func NewClient(clientID, clientSecret string) *Client {
	client := &Client{
		HTTPClient: http.Client{
			Timeout: time.Duration(5 * time.Second),
		},
		clientID:     clientID,
		clientSecret: clientSecret,
	}

	return client
}

// GetGithubUserAccessToken ...
func (c *Client) GetGithubUserAccessToken(code string) (token string, err error) {
	reqURL := fmt.Sprintf("https://github.com/login/oauth/access_token?client_id=%s&client_secret=%s&code=%s", c.clientID, c.clientSecret, code)
	req, err := http.NewRequest(http.MethodPost, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("could not create HTTP request: %v", err)
	}

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not send HTTP request: %v", err)
	}
	defer func() { _ = res.Body.Close() }()

	var t OAuthAccessResponse
	if err := json.NewDecoder(res.Body).Decode(&t); err != nil {
		return "", fmt.Errorf("could not parse JSON response: %v", err)
	}

	return t.AccessToken, nil
}

// GetGithubUser ...
func (c *Client) GetGithubUser(code string) (*UserResponse, error) {
	body, err := c.MakeRequest("https://api.github.com/user", code)
	if err != nil {
		return nil, err
	}

	var user UserResponse
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, err
	}

	return &user, nil
}

// MakeRequest ...
func (c *Client) MakeRequest(url, token string) ([]byte, error) {
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "token "+token)

	res, err := c.HTTPClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

// GetGithubUserRepos ...
func (c *Client) GetGithubUserRepos(code, username string) ([]*Repo, error) {
	var repos []*Repo

	url := "https://api.github.com/users/" + username + "/subscriptions"
	if body, err := c.MakeRequest(url, code); err == nil {
		if err2 := json.Unmarshal(body, &repos); err2 != nil {
			return nil, fmt.Errorf("%s\n%s\n%s\n%s", err2, string(body), "https://api.github.com/users/"+username+"/subscriptions", code)
		}
	}

	return repos, nil
}

// GetGithubUserRepoCommits ...
func (c *Client) GetGithubUserRepoCommits(item *database.UsersReposResult) ([]*CommitItem, error) {
	url := "https://api.github.com/repos/" + item.RepoName + "/commits?since=" + item.UpdatedAt.Add(time.Second*1).Format(time.RFC3339)

	body, err := c.MakeRequest(url, item.Token)
	if err != nil {
		return nil, err
	}

	var commits []*CommitItem
	if err := json.Unmarshal(body, &commits); err != nil {
		return nil, err
	}

	return commits, nil
}
