package postgres

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"auction_server/internal/domain"
)

func (r *AuctionRepository) ExtendAuction(auctionID, ownerID string, hours int) bool {
	if hours <= 0 {
		hours = 72
	}
	ctx := context.Background()
	now := time.Now().UnixMilli()
	add := int64(hours) * 60 * 60 * 1000
	tag, err := r.pool.Exec(ctx, `
UPDATE auctions
SET expires_at_ms = GREATEST(expires_at_ms, $3) + $4,
    status = 'ACTIVE',
    ended_at_ms = NULL
WHERE id = $1 AND owner_id = $2`, auctionID, ownerID, now, add)
	return err == nil && tag.RowsAffected() > 0
}

func (r *AuctionRepository) BumpAuction(auctionID, ownerID string) bool {
	ctx := context.Background()
	now := time.Now().UnixMilli()
	tag, err := r.pool.Exec(ctx, `
UPDATE auctions SET bumped_at_ms = $3
WHERE id = $1 AND owner_id = $2 AND status = 'ACTIVE' AND moderation_status = 'APPROVED'`, auctionID, ownerID, now)
	return err == nil && tag.RowsAffected() > 0
}

func (r *AuctionRepository) SetFeatured(auctionID, ownerID string) bool {
	ctx := context.Background()
	now := time.Now().UnixMilli()
	tag, err := r.pool.Exec(ctx, `
UPDATE auctions SET is_featured = true, bumped_at_ms = $3
WHERE id = $1 AND owner_id = $2 AND status = 'ACTIVE' AND moderation_status = 'APPROVED'`,
		auctionID, ownerID, now)
	return err == nil && tag.RowsAffected() > 0
}

func (r *AuctionRepository) DeleteAuction(auctionID, ownerID string) bool {
	ctx := context.Background()
	tag, err := r.pool.Exec(ctx, `DELETE FROM auctions WHERE id = $1 AND owner_id = $2`, auctionID, ownerID)
	return err == nil && tag.RowsAffected() > 0
}

func (r *AuctionRepository) UpdateAuction(auctionID string, input domain.UpdateAuctionInput) (domain.Auction, bool) {
	ctx := context.Background()
	if strings.TrimSpace(input.OwnerID) == "" ||
		strings.TrimSpace(input.Title) == "" ||
		strings.TrimSpace(input.Description) == "" {
		return domain.Auction{}, false
	}
	imgJSON, err := json.Marshal(input.ImageURLs)
	if err != nil {
		imgJSON = []byte("[]")
	}
	tag, err := r.pool.Exec(ctx, `
UPDATE auctions
SET title = $3, description = $4, image_urls = $5::jsonb, base_price = $6,
    moderation_status = 'PENDING', moderation_reason = '', is_featured = false
WHERE id = $1 AND owner_id = $2`,
		auctionID, input.OwnerID, input.Title, input.Description, string(imgJSON), input.BasePrice)
	if err != nil || tag.RowsAffected() == 0 {
		return domain.Auction{}, false
	}
	return r.GetAuctionByID(auctionID)
}

func (r *AuctionRepository) AuctionStats(auctionID, ownerID string) (domain.AuctionStats, bool) {
	ctx := context.Background()
	var owner string
	var views int
	err := r.pool.QueryRow(ctx, `
SELECT owner_id, COALESCE(view_count, 0) FROM auctions WHERE id = $1`, auctionID).Scan(&owner, &views)
	if err != nil || owner != ownerID {
		return domain.AuctionStats{}, false
	}
	var offerUsers int
	var offersCount int
	_ = r.pool.QueryRow(ctx, `
SELECT COUNT(DISTINCT NULLIF(user_id, ''))::int, COUNT(*)::int
FROM bids WHERE auction_id = $1`, auctionID).Scan(&offerUsers, &offersCount)
	return domain.AuctionStats{
		ViewCount:      views,
		OfferUserCount: offerUsers,
		OffersCount:    offersCount,
	}, true
}
