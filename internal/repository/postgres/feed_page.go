package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"auction_server/internal/domain"

	"github.com/jackc/pgx/v5"
)

func parseFeedCursor(cursor string) (sortMs int64, id string, ok bool) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0, "", false
	}
	parts := strings.SplitN(cursor, ":", 2)
	if len(parts) != 2 {
		return 0, "", false
	}
	ms, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || ms <= 0 || parts[1] == "" {
		return 0, "", false
	}
	return ms, parts[1], true
}

func makeFeedCursor(a domain.Auction) string {
	return fmt.Sprintf("%d:%s", sortKeyMs(a), a.ID)
}

func (r *AuctionRepository) ListAuctionsPage(filter domain.AuctionFeedFilter) domain.AuctionFeedPage {
	ctx := context.Background()
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	now := time.Now().UnixMilli()
	cursorMs, cursorID, hasCursor := parseFeedCursor(filter.Cursor)
	status := string(filter.Status)
	if status == "" {
		status = string(domain.AuctionStatusActive)
	}

	baseSelect := `
SELECT ` + auctionSelectSQL + `,
  (SELECT COUNT(*)::int FROM bids b WHERE b.auction_id = a.id) AS offers_count,
  COALESCE(NULLIF(bumped_at_ms, 0), created_at_ms) AS sort_ms
FROM auctions a`

	bidderID := strings.TrimSpace(filter.BidderUserID)
	bidderSQL := ""
	if bidderID != "" {
		bidderSQL = ` AND EXISTS (SELECT 1 FROM bids b WHERE b.auction_id = a.id AND b.user_id = $3)`
	}

	var (
		rows pgx.Rows
		err  error
	)
	if hasCursor {
		if bidderID != "" {
			rows, err = r.pool.Query(ctx, baseSelect+`
WHERE a.status = $1 AND a.expires_at_ms > $2`+publicFeedSQL+ bidderSQL+`
  AND (COALESCE(NULLIF(a.bumped_at_ms, 0), a.created_at_ms), a.id) < ($4::bigint, $5::text)
ORDER BY sort_ms DESC, a.id DESC
LIMIT $6`, status, now, bidderID, cursorMs, cursorID, limit+1)
		} else {
			rows, err = r.pool.Query(ctx, baseSelect+`
WHERE a.status = $1 AND a.expires_at_ms > $2`+publicFeedSQL+`
  AND (COALESCE(NULLIF(a.bumped_at_ms, 0), a.created_at_ms), a.id) < ($3::bigint, $4::text)
ORDER BY sort_ms DESC, a.id DESC
LIMIT $5`, status, now, cursorMs, cursorID, limit+1)
		}
	} else {
		if bidderID != "" {
			rows, err = r.pool.Query(ctx, baseSelect+`
WHERE a.status = $1 AND a.expires_at_ms > $2`+publicFeedSQL+ bidderSQL+`
ORDER BY sort_ms DESC, a.id DESC
LIMIT $4`, status, now, bidderID, limit+1)
		} else {
			rows, err = r.pool.Query(ctx, baseSelect+`
WHERE a.status = $1 AND a.expires_at_ms > $2`+publicFeedSQL+`
ORDER BY sort_ms DESC, a.id DESC
LIMIT $3`, status, now, limit+1)
		}
	}
	if err != nil {
		return domain.AuctionFeedPage{}
	}
	defer rows.Close()

	list := make([]domain.Auction, 0, limit+1)
	for rows.Next() {
		var sortMs int64
		a, err := scanAuctionFromRowWithSort(rows, &sortMs)
		if err != nil {
			return domain.AuctionFeedPage{}
		}
		_ = sortMs
		list = append(list, a)
	}
	if rows.Err() != nil {
		return domain.AuctionFeedPage{}
	}

	hasMore := len(list) > limit
	if hasMore {
		list = list[:limit]
	}
	r.attachTopBidSummaries(ctx, list)

	if filter.Sort == "trending" {
		sort.Slice(list, func(i, j int) bool {
			if list[i].OffersCount == list[j].OffersCount {
				return sortKeyMs(list[i]) > sortKeyMs(list[j])
			}
			return list[i].OffersCount > list[j].OffersCount
		})
	}

	var next string
	if hasMore && len(list) > 0 {
		next = makeFeedCursor(list[len(list)-1])
	}
	return domain.AuctionFeedPage{
		Items:      list,
		NextCursor: next,
		HasMore:    hasMore,
	}
}

func scanAuctionFromRowWithSort(row pgx.Row, sortMs *int64) (domain.Auction, error) {
	var (
		a           domain.Auction
		imgRaw      []byte
		offersCount int
		ended       sql.NullInt64
	)
	if err := row.Scan(
		&a.ID, &a.OwnerID, &a.Title, &a.Description, &imgRaw, &a.Status,
		&a.CreatedAtMs, &a.ExpiresAtMs, &ended, &a.MinBidStep, &a.BasePrice, &a.BumpedAtMs, &a.ViewCount,
		&a.IsFeatured, &a.ModerationStatus, &a.ModerationReason, &offersCount, sortMs,
	); err != nil {
		return domain.Auction{}, err
	}
	if ended.Valid {
		a.EndedAtMs = ended.Int64
	}
	_ = json.Unmarshal(imgRaw, &a.ImageURLs)
	if a.ImageURLs == nil {
		a.ImageURLs = []string{}
	}
	a.OffersCount = offersCount
	return a, nil
}
