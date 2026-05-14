package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"time"

	"auction_server/internal/domain"
	"auction_server/internal/repository"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ repository.AuctionRepository = (*AuctionRepository)(nil)

type AuctionRepository struct {
	pool *pgxpool.Pool
}

func NewAuctionRepository(pool *pgxpool.Pool) *AuctionRepository {
	return &AuctionRepository{pool: pool}
}

func (r *AuctionRepository) FinalizeExpired(nowMillis int64) {
	ctx := context.Background()
	_, _ = r.pool.Exec(ctx, `
UPDATE auctions SET status = 'ENDED', ended_at_ms = $1
WHERE status = 'ACTIVE' AND expires_at_ms <= $1`, nowMillis)
}

func (r *AuctionRepository) ListAuctions(filter domain.AuctionFeedFilter) []domain.Auction {
	ctx := context.Background()
	r.FinalizeExpired(time.Now().UnixMilli())

	var (
		rows pgx.Rows
		err  error
	)
	if filter.Status == "" {
		rows, err = r.pool.Query(ctx, `
SELECT id, owner_id, title, description, image_urls, status, created_at_ms, expires_at_ms, ended_at_ms,
  (SELECT COUNT(*)::int FROM bids b WHERE b.auction_id = a.id) AS offers_count
FROM auctions a`)
	} else {
		rows, err = r.pool.Query(ctx, `
SELECT id, owner_id, title, description, image_urls, status, created_at_ms, expires_at_ms, ended_at_ms,
  (SELECT COUNT(*)::int FROM bids b WHERE b.auction_id = a.id) AS offers_count
FROM auctions a WHERE a.status = $1`, string(filter.Status))
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
			return list[i].CreatedAtMs > list[j].CreatedAtMs
		})
	}
	return list
}

func (r *AuctionRepository) ListOwnerAuctions(ownerID string, status domain.AuctionStatus) []domain.Auction {
	ctx := context.Background()
	r.FinalizeExpired(time.Now().UnixMilli())

	var (
		rows pgx.Rows
		err  error
	)
	if status == "" {
		rows, err = r.pool.Query(ctx, `
SELECT id, owner_id, title, description, image_urls, status, created_at_ms, expires_at_ms, ended_at_ms,
  (SELECT COUNT(*)::int FROM bids b WHERE b.auction_id = a.id) AS offers_count
FROM auctions a WHERE a.owner_id = $1`, ownerID)
	} else {
		rows, err = r.pool.Query(ctx, `
SELECT id, owner_id, title, description, image_urls, status, created_at_ms, expires_at_ms, ended_at_ms,
  (SELECT COUNT(*)::int FROM bids b WHERE b.auction_id = a.id) AS offers_count
FROM auctions a WHERE a.owner_id = $1 AND a.status = $2`, ownerID, string(status))
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

func (r *AuctionRepository) GetAuctionByID(id string) (domain.Auction, bool) {
	ctx := context.Background()
	r.FinalizeExpired(time.Now().UnixMilli())

	row := r.pool.QueryRow(ctx, `
SELECT id, owner_id, title, description, image_urls, status, created_at_ms, expires_at_ms, ended_at_ms,
  (SELECT COUNT(*)::int FROM bids b WHERE b.auction_id = a.id) AS offers_count
FROM auctions a WHERE a.id = $1`, id)
	a, err := scanAuctionRowWithOffersCount(row)
	if err != nil {
		return domain.Auction{}, false
	}
	list := []domain.Auction{a}
	r.attachTopBidSummaries(ctx, list)
	return list[0], true
}

func (r *AuctionRepository) CreateAuction(input domain.CreateAuctionInput) domain.Auction {
	ctx := context.Background()
	durationHours := input.DurationHours
	if durationHours <= 0 {
		durationHours = 24
	}
	now := time.Now().UnixMilli()
	expires := now + int64(durationHours)*60*60*1000

	imgJSON, err := json.Marshal(input.ImageURLs)
	if err != nil {
		imgJSON = []byte("[]")
	}

	row := r.pool.QueryRow(ctx, `
INSERT INTO auctions (id, owner_id, title, description, image_urls, status, created_at_ms, expires_at_ms)
VALUES ('auction-' || nextval('auction_id_seq')::text, $1, $2, $3, $4::jsonb, 'ACTIVE', $5, $6)
RETURNING id, owner_id, title, description, image_urls, status, created_at_ms, expires_at_ms, ended_at_ms`,
		input.OwnerID, input.Title, input.Description, string(imgJSON), now, expires)

	a, err := scanAuctionRowInsert(row)
	if err != nil {
		return domain.Auction{}
	}
	return a
}

func (r *AuctionRepository) TopBids(auctionID string, limit int) []domain.Bid {
	ctx := context.Background()
	r.FinalizeExpired(time.Now().UnixMilli())

	if limit <= 0 {
		limit = 5
	}
	rows, err := r.pool.Query(ctx, `
SELECT auction_id, user_name, price, created_at_ms FROM bids
WHERE auction_id = $1
ORDER BY price DESC, created_at_ms ASC
LIMIT $2`, auctionID, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanBids(rows)
}

func (r *AuctionRepository) PlaceBid(auctionID string, input domain.PlaceBidInput) (domain.Bid, bool) {
	ctx := context.Background()
	r.FinalizeExpired(time.Now().UnixMilli())

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Bid{}, false
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var st string
	err = tx.QueryRow(ctx, `SELECT status FROM auctions WHERE id = $1 FOR UPDATE`, auctionID).Scan(&st)
	if err != nil {
		return domain.Bid{}, false
	}
	if st != string(domain.AuctionStatusActive) {
		return domain.Bid{}, false
	}

	now := time.Now().UnixMilli()
	_, err = tx.Exec(ctx, `
INSERT INTO bids (auction_id, user_name, price, created_at_ms) VALUES ($1, $2, $3, $4)`,
		auctionID, input.UserName, input.Price, now)
	if err != nil {
		return domain.Bid{}, false
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Bid{}, false
	}
	return domain.Bid{
		AuctionID: auctionID,
		UserName:  input.UserName,
		Price:     input.Price,
		CreatedAt: now,
	}, true
}

func (r *AuctionRepository) ResultContacts(auctionID, ownerID string, limit int) ([]domain.BidSummary, bool) {
	ctx := context.Background()
	r.FinalizeExpired(time.Now().UnixMilli())

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
SELECT user_name, price FROM bids WHERE auction_id = $1 ORDER BY price DESC LIMIT $2`, auctionID, limit)
	if err != nil {
		return nil, false
	}
	defer rows.Close()
	out := make([]domain.BidSummary, 0, limit)
	for rows.Next() {
		var s domain.BidSummary
		if err := rows.Scan(&s.UserName, &s.Price); err != nil {
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
		if err := rows.Scan(&b.AuctionID, &b.UserName, &b.Price, &b.CreatedAt); err != nil {
			continue
		}
		out = append(out, b)
	}
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
		&a.CreatedAtMs, &a.ExpiresAtMs, &ended,
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

func scanAuctionFromRow(row interface{ Scan(dest ...any) error }) (domain.Auction, error) {
	var (
		a           domain.Auction
		imgRaw      []byte
		offersCount int
		ended       sql.NullInt64
	)
	if err := row.Scan(
		&a.ID, &a.OwnerID, &a.Title, &a.Description, &imgRaw, &a.Status,
		&a.CreatedAtMs, &a.ExpiresAtMs, &ended, &offersCount,
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
		})
	}
}
