package cache

import (
	"context"

	"auction_server/internal/platform"
)

func InvalidateFeedCache() {
	if platform.Redis == nil {
		return
	}
	ctx := context.Background()
	iter := platform.Redis.Scan(ctx, 0, "feed:*", 200).Iterator()
	for iter.Next(ctx) {
		_ = platform.Redis.Del(ctx, iter.Val()).Err()
	}
}
