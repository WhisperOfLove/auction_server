package moderation

import (
	"context"
	"time"

	"auction_server/internal/platform"

	"github.com/redis/go-redis/v9"
)

const redisQueueKey = "moderation:pending"

// EnqueuePending adds an auction id to the Redis sorted set (score = submit time).
func EnqueuePending(auctionID string, createdAtMs int64) {
	if platform.Redis == nil || auctionID == "" {
		return
	}
	if createdAtMs <= 0 {
		createdAtMs = time.Now().UnixMilli()
	}
	ctx := context.Background()
	_ = platform.Redis.ZAdd(ctx, redisQueueKey, redis.Z{
		Score:  float64(createdAtMs),
		Member: auctionID,
	}).Err()
}

// DequeuePending removes an auction from the pending queue after a decision.
func DequeuePending(auctionID string) {
	if platform.Redis == nil || auctionID == "" {
		return
	}
	ctx := context.Background()
	_ = platform.Redis.ZRem(ctx, redisQueueKey, auctionID).Err()
}

// PendingCount returns queue size from Redis when available.
func PendingCount() int {
	if platform.Redis == nil {
		return -1
	}
	ctx := context.Background()
	n, err := platform.Redis.ZCard(ctx, redisQueueKey).Result()
	if err != nil {
		return -1
	}
	return int(n)
}
