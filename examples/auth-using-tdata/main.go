package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/sessionMaker"
	"github.com/gotd/td/session/tdesktop"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalln(err)
	}
	telegramDir := filepath.Join(home, ".local/share/TelegramDesktop")
	accounts, err := tdesktop.Read(telegramDir, nil)
	if err != nil {
		log.Fatalln(err)
	}

	client, err := gotgproto.NewClient(
		// Get AppID from https://my.telegram.org/apps
		123456,
		// Get ApiHash from https://my.telegram.org/apps
		"API_HASH_HERE",
		// ClientType, as we defined above
		gotgproto.ClientTypePhone("PHONE_NUMBER_HERE"),
		// Optional parameters of client
		&gotgproto.ClientOpts{
			// There can be up to 3 tdesktop.Account, we consider here there is
			// at least a single on, you can loop through them with
			// for _, account := range accounts {// your code}
			Session: sessionMaker.TdataSession(accounts[0]).Name("tdata"),
		},
	)
	if err != nil {
		log.Fatalln("failed to start client:", err)
	}

	fmt.Printf("client (@%s) has been started...\n", client.Self.Username)

	client.Idle()
}
