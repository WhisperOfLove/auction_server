package config

import "os"

type Config struct {
	Port string

	// Realtime intervals are configurable for admin ops.
	TopBidsPushIntervalSeconds string
	FeedRefreshIntervalSeconds string

	PostgresDSN string
	RedisAddr   string
	KafkaBrokers string
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func Load() Config {
	return Config{
		Port:                       env("APP_PORT", "8080"),
		TopBidsPushIntervalSeconds: env("TOP_BIDS_PUSH_INTERVAL_SECONDS", "10"),
		FeedRefreshIntervalSeconds: env("FEED_REFRESH_INTERVAL_SECONDS", "30"),
		PostgresDSN:                env("POSTGRES_DSN", "postgres://auction:auction@localhost:5432/auction?sslmode=disable"),
		RedisAddr:                  env("REDIS_ADDR", "localhost:6379"),
		KafkaBrokers:               env("KAFKA_BROKERS", "localhost:9092"),
	}
}
