package telegram

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"

	"golang.org/x/net/proxy"
	tgbotapi "gopkg.in/telegram-bot-api.v4"
)

// InitTelegram ...
func InitTelegram(token, proxyHost, proxyPort, proxyUser, proxyPassword string, debug bool) (bot *tgbotapi.BotAPI, err error) {
	var tr http.Transport

	if proxyHost != "" {
		tr = http.Transport{
			DialContext: func(_ context.Context, network, addr string) (net.Conn, error) {
				socksDialer, err := proxy.SOCKS5(
					"tcp",
					fmt.Sprintf("%s:%d", proxyHost, proxyPort),
					&proxy.Auth{User: proxyUser, Password: proxyPassword},
					proxy.Direct,
				)
				if err != nil {
					log.Println(err)
					return nil, err
				}

				return socksDialer.Dial(network, addr)
			},
		}
	}

	bot, err = tgbotapi.NewBotAPIWithClient(token, &http.Client{Transport: &tr})
	if err != nil {
		return nil, err
	}

	bot.Debug = debug

	log.Printf("Authorized on account @%s", bot.Self.UserName)

	return bot, nil
}
