package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port string

	TopBidsPushIntervalSeconds string
	FeedRefreshIntervalSeconds string

	FeedVisibleHours                int // hours post stays on main feed while ACTIVE
	MyAuctionsDeleteDaysAfterExpiry int // 0 = never auto-delete; N = days after expiry
	BidCooldownSeconds              int // minimum seconds between bids from same user on same auction

	PostgresDSN  string
	RedisAddr    string
	KafkaBrokers string

	AdminAPIKey string

	PublicBaseURL string
	UploadDir     string
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func envIntAllowZero(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

func Load() Config {
	return Config{
		Port:                       env("APP_PORT", "8080"),
		TopBidsPushIntervalSeconds: env("TOP_BIDS_PUSH_INTERVAL_SECONDS", "10"),
		FeedRefreshIntervalSeconds: env("FEED_REFRESH_INTERVAL_SECONDS", "30"),
		FeedVisibleHours: envInt("FEED_VISIBLE_HOURS", 72),
		MyAuctionsDeleteDaysAfterExpiry: envIntAllowZero("MY_AUCTIONS_DELETE_DAYS_AFTER_EXPIRY", 0),
		BidCooldownSeconds:            envIntAllowZero("BID_COOLDOWN_SECONDS", 300),
		PostgresDSN:                env("POSTGRES_DSN", ""),
		RedisAddr:                  env("REDIS_ADDR", ""),
		KafkaBrokers:               env("KAFKA_BROKERS", ""),
		AdminAPIKey:                env("ADMIN_API_KEY", ""),
		PublicBaseURL:              env("PUBLIC_BASE_URL", ""),
		UploadDir:                  env("UPLOAD_DIR", "uploads"),
	}
}
