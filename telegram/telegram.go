package telegram

import (
	"context"
	"fmt"
	"net"
	"net/http"

	dlog "github.com/amoghe/distillog"
	"golang.org/x/net/proxy"
	tgbotapi "gopkg.in/telegram-bot-api.v4"
)

// InitTelegram ...
func InitTelegram(token, proxyHost, proxyPort, proxyUser, proxyPassword string, debug bool) (bot *tgbotapi.BotAPI, err error) {
	var tr http.Transport

	if proxyHost != "" {
		tr = http.Transport{
			DialContext: func(_ context.Context, network, addr string) (net.Conn, error) {
				socksDialer, err2 := proxy.SOCKS5(
					"tcp",
					fmt.Sprintf("%s:%s", proxyHost, proxyPort),
					&proxy.Auth{User: proxyUser, Password: proxyPassword},
					proxy.Direct,
				)
				if err2 != nil {
					return nil, err2
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

	dlog.Debugf("Authorized on account @%s", bot.Self.UserName)

	return bot, nil
}
