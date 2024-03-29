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

// RepoErrorAnswer ...
type RepoErrorAnswer struct {
	Message          string `json:"message"`
	DocumentationURL string `json:"documentation_url"`
}

// CommitItem ...
type CommitItem struct {
	SHA     string `json:"sha"`
	Commit  Commit `json:"commit"`
	URL     string `json:"url"`
	HTMLUrl string `json:"html_url"`
}

// Commit ...
type Commit struct {
	Author    Author    `json:"author"`
	Message   string    `json:"message"`
	Committer Committer `json:"committer"`
}

// Author ...
type Author struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Date  time.Time `json:"date"`
}

// Committer ...
type Committer struct {
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

	req.Header.Set("Accept", "application/json")

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not send HTTP request: %v", err)
	}
	defer func() { _ = res.Body.Close() }()

	body, err2 := ioutil.ReadAll(res.Body)
	if err2 != nil {
		return "", err2
	}

	var t OAuthAccessResponse
	if err := json.Unmarshal(body, &t); err != nil {
		return "", fmt.Errorf("could not parse JSON response: %v\n%s", err, string(body))
	}

	return t.AccessToken, nil
}

// GetGithubUser ...
func (c *Client) GetGithubUser(code string) (*UserResponse, error) {
	url := "https://api.github.com/user"

	body, err := c.MakeRequest(url, code)
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
			return nil, fmt.Errorf("%s\n%s", err2, string(body))
		}
	} else {
		return nil, fmt.Errorf("%s\n%s", err, string(body))
	}

	return repos, nil
}

// GetGithubRepo ...
func (c *Client) GetGithubRepo(code, reponame string) (*Repo, error) {
	var repo *Repo

	url := "https://api.github.com/repos/" + reponame
	if body, err := c.MakeRequest(url, code); err == nil {
		if err2 := json.Unmarshal(body, &repo); err2 != nil {
			return nil, fmt.Errorf("%s\n%s", err2, string(body))
		}
	} else {
		return nil, fmt.Errorf("%s\n%s", err, string(body))
	}

	return repo, nil
}

// GetGithubUserRepoCommits ...
func (c *Client) GetGithubUserRepoCommits(item *database.UsersReposResult) ([]*CommitItem, error) {
	var commits []*CommitItem

	url := "https://api.github.com/repos/" + item.RepoName + "/commits?since=" + item.UpdatedAt.Add(time.Second*1).Format(time.RFC3339)
	if body, err := c.MakeRequest(url, item.Token); err == nil {
		if err2 := json.Unmarshal(body, &commits); err2 != nil {
			var repoErrorAnswer RepoErrorAnswer
			if err2 := json.Unmarshal(body, &repoErrorAnswer); err2 == nil {
				// {"message":"Not Found","documentation_url":"https://developer.github.com/v3/repos/commits/#list-commits-on-a-repository"}
				if repoErrorAnswer.Message == "Not Found" {
					return nil, fmt.Errorf("repo not found")
				}
				return nil, fmt.Errorf("%s\n%s", repoErrorAnswer.Message, string(body))
			}
			return nil, fmt.Errorf("%s\n%s", err2, string(body))
		}
	} else {
		return nil, fmt.Errorf("%s\n%s", err, string(body))
	}

	return commits, nil
}
