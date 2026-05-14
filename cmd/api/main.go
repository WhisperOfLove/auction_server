package main

import (
	"context"
	"log"
	"time"

	"auction_server/internal/config"
	httpserver "auction_server/internal/http"
	"auction_server/internal/repository"
	"auction_server/internal/repository/memory"
	"auction_server/internal/repository/postgres"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg := config.Load()
	repo := openRepository(cfg)
	srv := httpserver.NewServer(cfg, repo)
	log.Printf("auction server running on :%s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func openRepository(cfg config.Config) repository.AuctionRepository {
	if cfg.PostgresDSN == "" {
		log.Println("storage: in-memory (set POSTGRES_DSN for PostgreSQL)")
		return memory.NewAuctionRepository()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("postgres connect: %v", err)
	}
	if err := postgres.Migrate(ctx, pool); err != nil {
		log.Fatalf("postgres migrate: %v", err)
	}
	log.Println("storage: PostgreSQL")
	return postgres.NewAuctionRepository(pool)
}
