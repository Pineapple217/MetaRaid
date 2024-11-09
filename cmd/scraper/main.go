package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/Pineapple217/MetaRaid/pkg/config"
	"github.com/Pineapple217/MetaRaid/pkg/database"
	"github.com/Pineapple217/MetaRaid/pkg/helper"
	"github.com/Pineapple217/MetaRaid/pkg/scraper"
	"github.com/Pineapple217/MetaRaid/pkg/spotify"
)

const banner = `
   _____          __        __________        .__    .___
  /     \   _____/  |______ \______   \_____  |__| __| _/
 /  \ /  \_/ __ \   __\__  \ |       _/\__  \ |  |/ __ | 
/    Y    \  ___/|  |  / __ \|    |   \ / __ \|  / /_/ | 
\____|__  /\___  >__| (____  /____|_  /(____  /__\____ | 
        \/     \/          \/       \/      \/        \/ 

https://github.com/Pineapple217/MetaRaid
--------------------------------------------------------- `

func main() {
	slog.SetDefault(slog.New(slog.Default().Handler()))
	fmt.Println(banner)
	os.Stdout.Sync()

	conf, err := config.Load()
	helper.MaybeDie(err, "Failed to load configs")

	rdb := database.NewRedis(conf.Redis)
	clients := spotify.NewClient(conf.Spotify)

	s := scraper.NewScraper(clients, rdb, conf.Scraper)
	s.Start()
	defer s.Stop()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	slog.Info("Received an interrupt signal, exiting...")
}
