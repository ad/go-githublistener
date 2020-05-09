# go-githublistener

Register your new application on Github: https://github.com/settings/applications/new

In the "callback URL" field, enter http://localhost:8080/oauth/redirect

Once you register, you will get a client ID and client secret

Put clientID and clientSecret variables into .env file:

GO_GITHUB_LISTENER_PORT=8080

GO_GITHUB_CLIENT_ID=

GO_GITHUB_CLIENT_SECRET=


Start the server by executing make dev or make up

Navigate to http://localhost:8080 on your browser.