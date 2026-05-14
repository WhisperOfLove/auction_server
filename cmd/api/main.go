package main

import (
	"log"

	"auction_server/internal/config"
	httpserver "auction_server/internal/http"
)

func main() {
	cfg := config.Load()
	srv := httpserver.NewServer(cfg)
	log.Printf("auction server running on :%s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
