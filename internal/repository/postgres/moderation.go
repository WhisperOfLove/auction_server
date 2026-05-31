package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"auction_server/internal/domain"

	"github.com/jackc/pgx/v5"
)

const auctionSelectSQL = `
  a.id, a.owner_id, a.title, a.description, a.image_urls, a.status,
  a.created_at_ms, a.expires_at_ms, a.ended_at_ms,
  COALESCE(a.min_bid_step, 0), COALESCE(a.base_price, 0), COALESCE(a.bumped_at_ms, 0), COALESCE(a.view_count, 0),
  COALESCE(a.is_featured, false), COALESCE(a.moderation_status, 'APPROVED'), COALESCE(a.moderation_reason, '')`

const publicFeedSQL = ` AND a.moderation_status = 'APPROVED'`

func (r *AuctionRepository) CountModerationPending() int {
	ctx := context.Background()
	var n int
	err := r.pool.QueryRow(ctx, `
SELECT COUNT(*)::int FROM auctions WHERE moderation_status = 'PENDING'`).Scan(&n)
	if err != nil {
		return 0
	}
	return n
}

func (r *AuctionRepository) ListModerationQueue(filter domain.ModerationQueueFilter) domain.ModerationQueuePage {
	ctx := context.Background()
	status := filter.Status
	if status == "" {
		status = domain.ModerationPending
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	cursorMs, cursorID, hasCursor := parseFeedCursor(filter.Cursor)

	base := `
SELECT ` + auctionSelectSQL + `,
  (SELECT COUNT(*)::int FROM bids b WHERE b.auction_id = a.id) AS offers_count
FROM auctions a
WHERE a.moderation_status = $1`

	var (
		rows pgx.Rows
		err  error
	)
	if hasCursor {
		rows, err = r.pool.Query(ctx, base+`
  AND (a.created_at_ms, a.id) < ($2::bigint, $3::text)
ORDER BY a.created_at_ms DESC, a.id DESC
LIMIT $4`, string(status), cursorMs, cursorID, limit+1)
	} else {
		rows, err = r.pool.Query(ctx, base+`
ORDER BY a.created_at_ms DESC, a.id DESC
LIMIT $2`, string(status), limit+1)
	}
	if err != nil {
		return domain.ModerationQueuePage{}
	}
	defer rows.Close()

	list, err := scanAuctionsWithOffersCount(rows)
	if err != nil {
		return domain.ModerationQueuePage{}
	}
	hasMore := len(list) > limit
	if hasMore {
		list = list[:limit]
	}
	var next string
	if hasMore && len(list) > 0 {
		last := list[len(list)-1]
		next = fmt.Sprintf("%d:%s", last.CreatedAtMs, last.ID)
	}
	return domain.ModerationQueuePage{
		Items:        list,
		NextCursor:   next,
		HasMore:      hasMore,
		PendingCount: r.CountModerationPending(),
	}
}

func (r *AuctionRepository) ApproveModeration(id string) (domain.Auction, bool) {
	ctx := context.Background()
	now := time.Now().UnixMilli()
	row := r.pool.QueryRow(ctx, `
UPDATE auctions SET
  moderation_status = 'APPROVED',
  moderation_reason = '',
  moderated_at_ms = $2,
  created_at_ms = $2,
  bumped_at_ms = $2,
  status = 'ACTIVE',
  ended_at_ms = NULL,
  -- keep the original duration length, but restart the countdown from approval time
  expires_at_ms = $2 + GREATEST(expires_at_ms - created_at_ms, 3600000)
WHERE id = $1 AND moderation_status = 'PENDING'
RETURNING id, owner_id, title, description, image_urls, status, created_at_ms, expires_at_ms, ended_at_ms,
  COALESCE(min_bid_step, 0), COALESCE(base_price, 0), COALESCE(bumped_at_ms, 0), COALESCE(view_count, 0),
  COALESCE(is_featured, false), moderation_status, COALESCE(moderation_reason, '')`, id, now)
	a, err := scanAuctionRowInsert(row)
	if err != nil {
		return domain.Auction{}, false
	}
	return a, true
}

func (r *AuctionRepository) RejectModeration(id, reason string) (domain.Auction, bool) {
	ctx := context.Background()
	now := time.Now().UnixMilli()
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "مطابق قوانین حراج نیست."
	}
	row := r.pool.QueryRow(ctx, `
UPDATE auctions SET
  moderation_status = 'REJECTED',
  moderation_reason = $3,
  moderated_at_ms = $2,
  is_featured = false
WHERE id = $1 AND moderation_status = 'PENDING'
RETURNING id, owner_id, title, description, image_urls, status, created_at_ms, expires_at_ms, ended_at_ms,
  COALESCE(min_bid_step, 0), COALESCE(base_price, 0), COALESCE(bumped_at_ms, 0), COALESCE(view_count, 0),
  COALESCE(is_featured, false), moderation_status, COALESCE(moderation_reason, '')`, id, now, reason)
	a, err := scanAuctionRowInsert(row)
	if err != nil {
		return domain.Auction{}, false
	}
	return a, true
}
