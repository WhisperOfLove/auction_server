package jobs

import (
	"log"
	"time"

	"auction_server/internal/repository"
)

// StartExpiryJob marks ended auctions and optional cleanup on an interval (not per HTTP request).
func StartExpiryJob(repo repository.AuctionRepository, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			repo.FinalizeExpired(time.Now().UnixMilli())
		}
	}()
	log.Printf("expiry job started (every %s)", interval)
}
