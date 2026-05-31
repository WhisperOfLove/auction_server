package platform

import (
	"context"
	"log"
	"strings"

	"github.com/redis/go-redis/v9"
)

var Redis *redis.Client

func InitRedis(addr string) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		log.Println("redis: disabled (REDIS_ADDR empty)")
		return
	}
	Redis = redis.NewClient(&redis.Options{Addr: addr})
	ctx := context.Background()
	if err := Redis.Ping(ctx).Err(); err != nil {
		log.Printf("redis: ping failed (%v) — continuing without cache", err)
		Redis = nil
		return
	}
	log.Printf("redis: connected (%s)", addr)
}
