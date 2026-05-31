package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"sort"
	"strings"
	"time"

	"auction_server/internal/domain"
	"auction_server/internal/repository"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ repository.AuctionRepository = (*AuctionRepository)(nil)

type AuctionRepository struct {
	pool              *pgxpool.Pool
	feedVisibleHours                int
	deleteDaysAfterExpiry int
}

func NewAuctionRepository(pool *pgxpool.Pool, feedVisibleHours, deleteDaysAfterExpiry int) *AuctionRepository {
	if feedVisibleHours <= 0 {
		feedVisibleHours = 72
	}
	if deleteDaysAfterExpiry < 0 {
		deleteDaysAfterExpiry = 0
	}
	return &AuctionRepository{
		pool:                  pool,
		feedVisibleHours:      feedVisibleHours,
		deleteDaysAfterExpiry: deleteDaysAfterExpiry,
	}
}

func (r *AuctionRepository) FinalizeExpired(nowMillis int64) {
	ctx := context.Background()
	_, _ = r.pool.Exec(ctx, `
UPDATE auctions SET status = 'ENDED', ended_at_ms = $2
WHERE status = 'ACTIVE' AND moderation_status = 'APPROVED' AND expires_at_ms > 0 AND expires_at_ms <= $1`, nowMillis, nowMillis)
	if r.deleteDaysAfterExpiry > 0 {
		cutoff := nowMillis - int64(r.deleteDaysAfterExpiry)*24*60*60*1000
		_, _ = r.pool.Exec(ctx, `
DELETE FROM auctions
WHERE status = 'ENDED' AND moderation_status = 'APPROVED' AND ended_at_ms > 0 AND ended_at_ms <= $1`, cutoff)
	}
}

func (r *AuctionRepository) ListAuctions(filter domain.AuctionFeedFilter) []domain.Auction {
	ctx := context.Background()

	var (
		rows pgx.Rows
		err  error
	)
	if filter.Status == "" {
		rows, err = r.pool.Query(ctx, `
SELECT `+auctionSelectSQL+`,
  (SELECT COUNT(*)::int FROM bids b WHERE b.auction_id = a.id) AS offers_count
FROM auctions a`)
	} else {
		now := time.Now().UnixMilli()
		rows, err = r.pool.Query(ctx, `
SELECT `+auctionSelectSQL+`,
  (SELECT COUNT(*)::int FROM bids b WHERE b.auction_id = a.id) AS offers_count
FROM auctions a
WHERE a.status = $1 AND a.expires_at_ms > $2`+publicFeedSQL, string(filter.Status), now)
	}
	if err != nil {
		return nil
	}
	defer rows.Close()

	list, err := scanAuctionsWithOffersCount(rows)
	if err != nil {
		return nil
	}
	r.attachTopBidSummaries(ctx, list)

	if filter.Sort == "trending" {
		sort.Slice(list, func(i, j int) bool {
			if list[i].OffersCount == list[j].OffersCount {
				return list[i].CreatedAtMs > list[j].CreatedAtMs
			}
			return list[i].OffersCount > list[j].OffersCount
		})
	} else {
		sort.Slice(list, func(i, j int) bool {
			if sortKeyMs(list[i]) != sortKeyMs(list[j]) {
				return sortKeyMs(list[i]) > sortKeyMs(list[j])
			}
			return list[i].ID > list[j].ID
		})
	}
	return list
}

func (r *AuctionRepository) ListOwnerAuctions(ownerID string, status domain.AuctionStatus) []domain.Auction {
	ctx := context.Background()
	var (
		rows pgx.Rows
		err  error
	)
	if status == "" {
		rows, err = r.pool.Query(ctx, `
SELECT `+auctionSelectSQL+`,
  (SELECT COUNT(*)::int FROM bids b WHERE b.auction_id = a.id) AS offers_count
FROM auctions a WHERE a.owner_id = $1`, ownerID)
	} else {
		rows, err = r.pool.Query(ctx, `
SELECT `+auctionSelectSQL+`,
  (SELECT COUNT(*)::int FROM bids b WHERE b.auction_id = a.id) AS offers_count
FROM auctions a WHERE a.owner_id = $1 AND a.status = $2`,
			ownerID, string(status))
	}
	if err != nil {
		return nil
	}
	defer rows.Close()

	list, err := scanAuctionsWithOffersCount(rows)
	if err != nil {
		return nil
	}
	r.attachTopBidSummaries(ctx, list)
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAtMs > list[j].CreatedAtMs
	})
	return list
}

func (r *AuctionRepository) PostPeriodCounts() []domain.AdminPeriodCount {
	ctx := context.Background()
	now := time.Now().UnixMilli()
	day := now - 24*60*60*1000
	week := now - 7*24*60*60*1000
	month := now - 30*24*60*60*1000
	var d, w, m int64
	_ = r.pool.QueryRow(ctx, `
SELECT
  COUNT(*) FILTER (WHERE created_at_ms >= $1)::bigint AS day_count,
  COUNT(*) FILTER (WHERE created_at_ms >= $2)::bigint AS week_count,
  COUNT(*) FILTER (WHERE created_at_ms >= $3)::bigint AS month_count
FROM auctions`, day, week, month).Scan(&d, &w, &m)
	return []domain.AdminPeriodCount{
		{Label: "روز", Value: d},
		{Label: "هفته", Value: w},
		{Label: "ماه", Value: m},
	}
}

func (r *AuctionRepository) GetAuctionByID(id string) (domain.Auction, bool) {
	ctx := context.Background()
	_, _ = r.pool.Exec(ctx, `UPDATE auctions SET view_count = view_count + 1 WHERE id = $1`, id)

	row := r.pool.QueryRow(ctx, `
SELECT `+auctionSelectSQL+`,
  (SELECT COUNT(*)::int FROM bids b WHERE b.auction_id = a.id) AS offers_count
FROM auctions a WHERE a.id = $1`, id)
	a, err := scanAuctionRowWithOffersCount(row)
	if err != nil {
		return domain.Auction{}, false
	}
	list := []domain.Auction{a}
	r.attachTopBidSummaries(ctx, list)
	a = list[0]
	a.TopBids = r.TopBids(id, 5)
	return a, true
}

func (r *AuctionRepository) CreateAuction(input domain.CreateAuctionInput) domain.Auction {
	ctx := context.Background()
	durationHours := input.DurationHours
	if durationHours <= 0 {
		durationHours = 72
	}
	now := time.Now().UnixMilli()
	expires := now + int64(durationHours)*60*60*1000

	imgJSON, err := json.Marshal(input.ImageURLs)
	if err != nil {
		imgJSON = []byte("[]")
	}

	basePrice := input.BasePrice
	if basePrice < 0 {
		basePrice = 0
	}
	row := r.pool.QueryRow(ctx, `
INSERT INTO auctions (id, owner_id, title, description, image_urls, status, created_at_ms, expires_at_ms, base_price, bumped_at_ms, moderation_status, duration_hours)
VALUES ('auction-' || nextval('auction_id_seq')::text, $1, $2, $3, $4::jsonb, 'ACTIVE', $5, $6, $7, $5, 'PENDING', $8)
RETURNING id, owner_id, title, description, image_urls, status, created_at_ms, expires_at_ms, ended_at_ms,
  COALESCE(min_bid_step, 0), COALESCE(base_price, 0), COALESCE(bumped_at_ms, 0), COALESCE(view_count, 0),
  COALESCE(is_featured, false), moderation_status, COALESCE(moderation_reason, '')`,
		input.OwnerID, input.Title, input.Description, string(imgJSON), now, expires, basePrice, durationHours)

	a, err := scanAuctionRowInsert(row)
	if err != nil {
		return domain.Auction{}
	}
	return a
}

func (r *AuctionRepository) TopBids(auctionID string, limit int) []domain.Bid {
	ctx := context.Background()
	if limit <= 0 {
		limit = 5
	}
	rows, err := r.pool.Query(ctx, `
SELECT b.auction_id,
  COALESCE(NULLIF(b.user_id, ''), ''),
  b.user_name,
  CASE WHEN COALESCE(u.receive_calls, TRUE)
    THEN COALESCE(NULLIF(b.phone, ''), u.phone, '')
    ELSE '' END,
  COALESCE(NULLIF(u.gender, ''), ''),
  COALESCE(
    NULLIF(TRIM(u.display_name), ''),
    CASE WHEN COALESCE(u.user_number, 0) > 0 THEN u.user_number::text ELSE '' END
  ),
  COALESCE(u.receive_calls, TRUE),
  b.price,
  b.created_at_ms
FROM bids b
LEFT JOIN users u ON u.id = b.user_id
WHERE b.auction_id = $1
ORDER BY b.price DESC, b.created_at_ms ASC
LIMIT $2`, auctionID, limit)
	if err != nil {
		return []domain.Bid{}
	}
	defer rows.Close()
	return scanBids(rows)
}

func (r *AuctionRepository) LastBidAtMs(auctionID, userID string) (int64, bool) {
	ctx := context.Background()
	auctionID = strings.TrimSpace(auctionID)
	userID = strings.TrimSpace(userID)
	if auctionID == "" || userID == "" {
		return 0, false
	}
	var at int64
	err := r.pool.QueryRow(ctx, `
SELECT last_bid_at_ms
FROM bid_cooldowns
WHERE auction_id = $1 AND user_id = $2`, auctionID, userID).Scan(&at)
	if err == nil && at > 0 {
		return at, true
	}
	err = r.pool.QueryRow(ctx, `
SELECT COALESCE(MAX(created_at_ms), 0)
FROM bids
WHERE auction_id = $1 AND user_id = $2`, auctionID, userID).Scan(&at)
	if err != nil {
		return 0, false
	}
	return at, true
}

// roundBidStep turns 1% of a bid into a human-friendly fixed increment (e.g. 1_232_159 → 1_232_000).
func roundBidStep(onePercent int64) int64 {
	if onePercent < 1 {
		return 1
	}
	if onePercent < 1000 {
		return onePercent
	}
	if onePercent < 100_000 {
		return (onePercent / 1000) * 1000
	}
	if onePercent < 1_000_000 {
		return (onePercent / 10_000) * 10_000
	}
	return (onePercent / 100_000) * 100_000
}

func (r *AuctionRepository) firstBidPrice(ctx context.Context, auctionID string) int64 {
	var price sql.NullInt64
	err := r.pool.QueryRow(ctx,
		`SELECT price FROM bids WHERE auction_id = $1 ORDER BY created_at_ms ASC LIMIT 1`,
		auctionID).Scan(&price)
	if err != nil || !price.Valid {
		return 0
	}
	return price.Int64
}

// lockMinBidStep saves increment from the first offer only (1% of that price, rounded).
func (r *AuctionRepository) lockMinBidStep(ctx context.Context, auctionID string, firstOfferPrice int64) int64 {
	step := roundBidStep(firstOfferPrice / 100)
	if step < 1 {
		step = 1
	}
	_, _ = r.pool.Exec(ctx,
		`UPDATE auctions SET min_bid_step = $1 WHERE id = $2 AND min_bid_step = 0`,
		step, auctionID)
	return step
}

func (r *AuctionRepository) MinRequiredBid(auctionID string) (int64, bool) {
	ctx := context.Background()
	var minStep int64
	var basePrice int64
	var top sql.NullInt64
	err := r.pool.QueryRow(ctx, `
SELECT COALESCE(a.min_bid_step, 0), COALESCE(a.base_price, 0),
  (SELECT MAX(b.price) FROM bids b WHERE b.auction_id = a.id)
FROM auctions a WHERE a.id = $1`, auctionID).Scan(&minStep, &basePrice, &top)
	if err != nil {
		return 0, false
	}
	if !top.Valid || top.Int64 <= 0 {
		if basePrice > 0 {
			return basePrice, true
		}
		return 1, true
	}
	step := minStep
	if step <= 0 {
		first := r.firstBidPrice(ctx, auctionID)
		if first > 0 {
			step = r.lockMinBidStep(ctx, auctionID, first)
		}
	}
	if step <= 0 {
		step = 1
	}
	return top.Int64 + step, true
}

func (r *AuctionRepository) PlaceBid(auctionID string, input domain.PlaceBidInput) (domain.Bid, bool) {
	ctx := context.Background()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Bid{}, false
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var st string
	var expiresAt int64
	err = tx.QueryRow(ctx, `SELECT status, expires_at_ms FROM auctions WHERE id = $1 FOR UPDATE`, auctionID).Scan(&st, &expiresAt)
	if err != nil {
		log.Printf("[bid] auction not found id=%s: %v", auctionID, err)
		return domain.Bid{}, false
	}
	if st != string(domain.AuctionStatusActive) {
		log.Printf("[bid] auction not active id=%s status=%s", auctionID, st)
		return domain.Bid{}, false
	}
	now := time.Now().UnixMilli()
	if expiresAt > 0 && expiresAt <= now {
		log.Printf("[bid] auction expired id=%s expires_at_ms=%d now=%d", auctionID, expiresAt, now)
		return domain.Bid{}, false
	}

	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		userID = "anon-" + strings.TrimSpace(input.Phone)
	}
	if userID == "" {
		userID = "anon-" + strings.TrimSpace(input.UserName)
	}
	var bidCount int
	_ = tx.QueryRow(ctx, `SELECT COUNT(*) FROM bids WHERE auction_id = $1`, auctionID).Scan(&bidCount)
	_, err = tx.Exec(ctx, `
INSERT INTO bids (auction_id, user_id, user_name, phone, price, created_at_ms) VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (auction_id, user_id) DO UPDATE SET
  user_name = EXCLUDED.user_name,
  phone = EXCLUDED.phone,
  price = EXCLUDED.price,
  created_at_ms = EXCLUDED.created_at_ms`,
		auctionID, userID, input.UserName, input.Phone, input.Price, now)
	if err != nil {
		log.Printf("[bid] insert failed id=%s user=%s price=%d: %v", auctionID, userID, input.Price, err)
		return domain.Bid{}, false
	}
	if bidCount == 0 {
		step := roundBidStep(input.Price / 100)
		if step < 1 {
			step = 1
		}
		_, _ = tx.Exec(ctx,
			`UPDATE auctions SET min_bid_step = $1 WHERE id = $2 AND min_bid_step = 0`,
			step, auctionID)
	}
	_, _ = tx.Exec(ctx, `
INSERT INTO bid_cooldowns (auction_id, user_id, last_bid_at_ms)
VALUES ($1, $2, $3)
ON CONFLICT (auction_id, user_id) DO UPDATE SET last_bid_at_ms = EXCLUDED.last_bid_at_ms`,
		auctionID, userID, now)
	if err := tx.Commit(ctx); err != nil {
		return domain.Bid{}, false
	}
	return domain.Bid{
		AuctionID: auctionID,
		UserID:    userID,
		UserName:  input.UserName,
		Phone:     input.Phone,
		Price:     input.Price,
		CreatedAt: now,
	}, true
}

func (r *AuctionRepository) DeleteBid(auctionID, userID string) bool {
	ctx := context.Background()
	auctionID = strings.TrimSpace(auctionID)
	userID = strings.TrimSpace(userID)
	if auctionID == "" || userID == "" {
		return false
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM bids WHERE auction_id = $1 AND user_id = $2`, auctionID, userID)
	return err == nil && tag.RowsAffected() > 0
}

func (r *AuctionRepository) ResultContacts(auctionID, ownerID string, limit int) ([]domain.BidSummary, bool) {
	ctx := context.Background()
	var owner string
	var st string
	err := r.pool.QueryRow(ctx, `SELECT owner_id, status FROM auctions WHERE id = $1`, auctionID).Scan(&owner, &st)
	if err != nil || owner != ownerID || st != string(domain.AuctionStatusEnded) {
		return nil, false
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx, `
SELECT user_name, phone, price FROM bids WHERE auction_id = $1 ORDER BY price DESC LIMIT $2`, auctionID, limit)
	if err != nil {
		return nil, false
	}
	defer rows.Close()
	out := make([]domain.BidSummary, 0, limit)
	for rows.Next() {
		var s domain.BidSummary
		if err := rows.Scan(&s.UserName, &s.Phone, &s.Price); err != nil {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

func scanBids(rows pgx.Rows) []domain.Bid {
	out := make([]domain.Bid, 0)
	for rows.Next() {
		var b domain.Bid
		if err := rows.Scan(
			&b.AuctionID, &b.UserID, &b.UserName, &b.Phone,
			&b.Gender, &b.FamilyName, &b.ReceiveCalls, &b.Price, &b.CreatedAt,
		); err != nil {
			continue
		}
		out = append(out, b)
	}
	_ = rows.Err()
	return out
}

func scanAuctionsWithOffersCount(rows pgx.Rows) ([]domain.Auction, error) {
	list := make([]domain.Auction, 0)
	for rows.Next() {
		a, err := scanAuctionFromRow(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func scanAuctionRowWithOffersCount(row pgx.Row) (domain.Auction, error) {
	return scanAuctionFromRow(row)
}

func scanAuctionRowInsert(row pgx.Row) (domain.Auction, error) {
	var (
		a      domain.Auction
		imgRaw []byte
		ended  sql.NullInt64
	)
	if err := row.Scan(
		&a.ID, &a.OwnerID, &a.Title, &a.Description, &imgRaw, &a.Status,
		&a.CreatedAtMs, &a.ExpiresAtMs, &ended, &a.MinBidStep, &a.BasePrice, &a.BumpedAtMs, &a.ViewCount,
		&a.IsFeatured, &a.ModerationStatus, &a.ModerationReason,
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
	a.OffersCount = 0
	return a, nil
}

func sortKeyMs(a domain.Auction) int64 {
	if a.BumpedAtMs > 0 {
		return a.BumpedAtMs
	}
	return a.CreatedAtMs
}

func scanAuctionFromRow(row interface{ Scan(dest ...any) error }) (domain.Auction, error) {
	var (
		a           domain.Auction
		imgRaw      []byte
		offersCount int
		ended       sql.NullInt64
	)
	if err := row.Scan(
		&a.ID, &a.OwnerID, &a.Title, &a.Description, &imgRaw, &a.Status,
		&a.CreatedAtMs, &a.ExpiresAtMs, &ended, &a.MinBidStep, &a.BasePrice, &a.BumpedAtMs, &a.ViewCount,
		&a.IsFeatured, &a.ModerationStatus, &a.ModerationReason, &offersCount,
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

func (r *AuctionRepository) attachTopBidSummaries(ctx context.Context, list []domain.Auction) {
	if len(list) == 0 {
		return
	}
	ids := make([]string, len(list))
	idIndex := make(map[string]int, len(list))
	for i := range list {
		ids[i] = list[i].ID
		idIndex[list[i].ID] = i
	}
	rows, err := r.pool.Query(ctx, `
SELECT auction_id, user_name, price FROM (
  SELECT auction_id, user_name, price,
    ROW_NUMBER() OVER (PARTITION BY auction_id ORDER BY price DESC, created_at_ms ASC) AS rn
  FROM bids WHERE auction_id = ANY($1::text[])
) t WHERE rn <= 5`, ids)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var aid, user string
		var price int64
		if err := rows.Scan(&aid, &user, &price); err != nil {
			continue
		}
		idx, ok := idIndex[aid]
		if !ok {
			continue
		}
		list[idx].FinalTopOffers = append(list[idx].FinalTopOffers, domain.BidSummary{
			UserName: user,
			Price:    price,
		}) // phone filled via top-bids endpoint for detail views
	}
}
