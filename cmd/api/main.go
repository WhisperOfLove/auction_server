package main

import (
	"context"
	"log"
	"path/filepath"
	"time"

	"auction_server/internal/appsettings"
	"auction_server/internal/config"
	"auction_server/internal/events"
	httpserver "auction_server/internal/http"
	"auction_server/internal/jobs"
	"auction_server/internal/platform"
	"auction_server/internal/repository"
	"auction_server/internal/repository/memory"
	"auction_server/internal/repository/postgres"
	"auction_server/internal/service"
	"auction_server/internal/ws"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg := config.Load()
	platform.InitRedis(cfg.RedisAddr)
	events.InitKafka(cfg.KafkaBrokers)
	defer events.Close()

	repo, users, chat, notifs, pool := openStorage(cfg, cfg.FeedVisibleHours, cfg.MyAuctionsDeleteDaysAfterExpiry)
	jobs.StartExpiryJob(repo, time.Minute)

	hub := ws.NewHub(cfg.AdminAPIKey)
	moderationSvc := service.NewModerationService(repo, notifs, chat, hub)
	defaults := appsettings.DefaultsFromConfig(cfg)
	var settingsStore appsettings.Provider
	if pool != nil {
		settingsStore = postgres.NewAppSettingsRepository(pool, defaults)
		log.Printf("app_settings: PostgreSQL (table app_settings)")
	} else {
		settingsPath := filepath.Join(cfg.UploadDir, "..", "app_settings.json")
		if cfg.UploadDir == "uploads" {
			settingsPath = "app_settings.json"
		}
		settingsStore = appsettings.NewStore(settingsPath, defaults)
		log.Printf("app_settings: file %s", settingsPath)
	}
	srv := httpserver.NewServer(cfg, repo, users, chat, hub, moderationSvc, settingsStore)
	log.Printf("auction server running on :%s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func openStorage(cfg config.Config, feedHours, deleteDaysAfterExpiry int) (repository.AuctionRepository, *postgres.UserRepository, *postgres.ChatRepository, *postgres.NotificationRepository, *pgxpool.Pool) {
	if cfg.PostgresDSN == "" {
		log.Println("storage: in-memory (set POSTGRES_DSN for PostgreSQL)")
		return memory.NewAuctionRepository(deleteDaysAfterExpiry), nil, nil, nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	poolCfg, err := pgxpool.ParseConfig(cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("postgres config: %v", err)
	}
	poolCfg.MaxConns = 64
	poolCfg.MinConns = 4
	poolCfg.MaxConnLifetime = time.Hour
	poolCfg.MaxConnIdleTime = 30 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		log.Fatalf("postgres connect: %v", err)
	}
	if err := postgres.Migrate(ctx, pool); err != nil {
		log.Fatalf("postgres migrate: %v", err)
	}
	log.Println("storage: PostgreSQL (pool max=64)")
	return postgres.NewAuctionRepository(pool, feedHours, deleteDaysAfterExpiry),
		postgres.NewUserRepository(pool),
		postgres.NewChatRepository(pool),
		postgres.NewNotificationRepository(pool),
		pool
}
