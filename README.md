# go-githublistener

Register your new application on Github: https://github.com/settings/applications/new

In the "callback URL" field, enter http://localhost:8080/oauth/redirect

Once you register, you will get a client ID and client secret

Replace the values of the clientID and clientSecret variables in the main.go file and also the index.html file

Start the server by executing go run main.go

Navigate to http://localhost:8080 on your browser.