package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"auction_server/internal/domain"
	"auction_server/internal/platform"

	"github.com/redis/go-redis/v9"
)

const topBidsTTL = 30 * time.Second

func topKey(auctionID string) string { return "auction:" + auctionID + ":topbids" }

func pubKey(auctionID string) string { return "auction:" + auctionID + ":bids" }

// SetTopBids stores serialized top bids for fast reads.
func SetTopBids(auctionID string, bids []domain.Bid) {
	if platform.Redis == nil || auctionID == "" {
		return
	}
	ctx := context.Background()
	raw, err := json.Marshal(bids)
	if err != nil {
		return
	}
	_ = platform.Redis.Set(ctx, topKey(auctionID), raw, topBidsTTL).Err()
	_ = platform.Redis.Publish(ctx, pubKey(auctionID), "1").Err()
}

// GetTopBids returns cached top bids if present.
func GetTopBids(auctionID string) ([]domain.Bid, bool) {
	if platform.Redis == nil || auctionID == "" {
		return nil, false
	}
	ctx := context.Background()
	raw, err := platform.Redis.Get(ctx, topKey(auctionID)).Bytes()
	if err != nil {
		return nil, false
	}
	var bids []domain.Bid
	if json.Unmarshal(raw, &bids) != nil {
		return nil, false
	}
	return bids, true
}

// SubscribeBidUpdates listens for bid changes (used by WebSocket hub).
func SubscribeBidUpdates(ctx context.Context, auctionID string) *redis.PubSub {
	if platform.Redis == nil {
		return nil
	}
	return platform.Redis.Subscribe(ctx, pubKey(auctionID))
}

func InvalidateTopBids(auctionID string) {
	if platform.Redis == nil {
		return
	}
	ctx := context.Background()
	_ = platform.Redis.Del(ctx, topKey(auctionID)).Err()
}

func FeedCacheKey(status, sort, cursor string, limit int, bidderUserID string) string {
	return fmt.Sprintf("feed:%s:%s:%s:%d:%s", status, sort, cursor, limit, bidderUserID)
}

func GetFeedPage(key string) (*domain.AuctionFeedPage, bool) {
	if platform.Redis == nil {
		return nil, false
	}
	ctx := context.Background()
	raw, err := platform.Redis.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	var page domain.AuctionFeedPage
	if json.Unmarshal(raw, &page) != nil {
		return nil, false
	}
	return &page, true
}

func SetFeedPage(key string, page domain.AuctionFeedPage) {
	if platform.Redis == nil {
		return
	}
	ctx := context.Background()
	raw, _ := json.Marshal(page)
	_ = platform.Redis.Set(ctx, key, raw, 15*time.Second).Err()
}
