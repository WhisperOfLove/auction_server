package postgres

import (
	"context"
	"time"

	"auction_server/internal/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

type NotificationRepository struct {
	pool *pgxpool.Pool
}

func NewNotificationRepository(pool *pgxpool.Pool) *NotificationRepository {
	return &NotificationRepository{pool: pool}
}

func (r *NotificationRepository) Create(n domain.UserNotification) (domain.UserNotification, bool) {
	if r == nil || r.pool == nil || n.UserID == "" {
		return domain.UserNotification{}, false
	}
	ctx := context.Background()
	if n.CreatedAtMs == 0 {
		n.CreatedAtMs = time.Now().UnixMilli()
	}
	err := r.pool.QueryRow(ctx, `
INSERT INTO user_notifications (user_id, kind, title, body, ref_auction_id, created_at_ms)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id`, n.UserID, n.Kind, n.Title, n.Body, n.RefAuctionID, n.CreatedAtMs).Scan(&n.ID)
	if err != nil {
		return domain.UserNotification{}, false
	}
	return n, true
}

func (r *NotificationRepository) ListForUser(userID string, limit int) []domain.UserNotification {
	if r == nil || r.pool == nil || userID == "" {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	ctx := context.Background()
	rows, err := r.pool.Query(ctx, `
SELECT id, user_id, kind, title, body, ref_auction_id, COALESCE(read_at_ms, 0), created_at_ms
FROM user_notifications
WHERE user_id = $1
ORDER BY created_at_ms DESC
LIMIT $2`, userID, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]domain.UserNotification, 0)
	for rows.Next() {
		var n domain.UserNotification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Kind, &n.Title, &n.Body, &n.RefAuctionID, &n.ReadAtMs, &n.CreatedAtMs); err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

func (r *NotificationRepository) MarkRead(userID string, id int64) bool {
	if r == nil || r.pool == nil {
		return false
	}
	ctx := context.Background()
	tag, err := r.pool.Exec(ctx, `
UPDATE user_notifications SET read_at_ms = $3
WHERE id = $1 AND user_id = $2 AND read_at_ms IS NULL`, id, userID, time.Now().UnixMilli())
	if err != nil {
		return false
	}
	return tag.RowsAffected() > 0
}
