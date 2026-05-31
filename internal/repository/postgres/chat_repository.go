package postgres

import (
	"context"
	"sort"
	"strings"
	"time"

	"auction_server/internal/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Deprecated: use domain.SupportAuctionID
const SupportAuctionID = "__support__"

type ChatRepository struct {
	pool *pgxpool.Pool
}

func NewChatRepository(pool *pgxpool.Pool) *ChatRepository {
	return &ChatRepository{pool: pool}
}

func (r *ChatRepository) ListMessages(auctionID, viewerID, peerID string, limit int) []domain.ChatMessage {
	if limit <= 0 {
		limit = 100
	}
	ctx := context.Background()
	viewerID = strings.TrimSpace(viewerID)
	peerID = strings.TrimSpace(peerID)

	var (
		rows interface {
			Close()
			Next() bool
			Scan(dest ...any) error
		}
		err error
	)

	if viewerID != "" && peerID != "" {
		rows, err = r.pool.Query(ctx, `
SELECT id, auction_id, sender_id, peer_id, sender_name, body, attachment_url, created_at_ms
FROM chat_messages
WHERE auction_id = $1
  AND ((sender_id = $2 AND peer_id = $3) OR (sender_id = $3 AND peer_id = $2))
ORDER BY created_at_ms ASC
LIMIT $4`, auctionID, viewerID, peerID, limit)
	} else {
		rows, err = r.pool.Query(ctx, `
SELECT id, auction_id, sender_id, peer_id, sender_name, body, attachment_url, created_at_ms
FROM chat_messages WHERE auction_id = $1
ORDER BY created_at_ms ASC
LIMIT $2`, auctionID, limit)
	}
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]domain.ChatMessage, 0)
	for rows.Next() {
		var m domain.ChatMessage
		if err := rows.Scan(
			&m.ID, &m.AuctionID, &m.SenderID, &m.PeerID, &m.SenderName,
			&m.Body, &m.AttachmentURL, &m.CreatedAtMs,
		); err != nil {
			continue
		}
		out = append(out, m)
	}
	return out
}

func (r *ChatRepository) ListInbox(userID string, limit int) []domain.ChatThread {
	if limit <= 0 {
		limit = 50
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return []domain.ChatThread{}
	}
	ctx := context.Background()
	rows, err := r.pool.Query(ctx, `
SELECT DISTINCT ON (t.auction_id, t.other_peer)
  t.auction_id,
  t.auction_owner_id,
  t.other_peer,
  COALESCE(
    NULLIF(TRIM(u.display_name), ''),
    CASE WHEN COALESCE(u.user_number, 0) > 0 THEN u.user_number::text ELSE NULL END,
    NULLIF(u.username, ''),
    t.other_peer
  ) AS peer_name,
  CASE WHEN COALESCE(u.receive_calls, TRUE)
    THEN COALESCE(NULLIF(u.phone, ''), '')
    ELSE '' END AS peer_phone,
  COALESCE(u.receive_calls, TRUE) AS peer_receive_calls,
  COALESCE(NULLIF(u.gender, ''), '') AS peer_gender,
  COALESCE(
    NULLIF(TRIM(u.display_name), ''),
    CASE WHEN COALESCE(u.user_number, 0) > 0 THEN u.user_number::text ELSE '' END
  ) AS peer_family,
  COALESCE(NULLIF(u.avatar_key, ''), '') AS peer_avatar,
  t.body,
  t.created_at_ms,
  t.last_sender_id
FROM (
  SELECT
    auction_id,
    auction_owner_id,
    CASE WHEN sender_id = $1 THEN peer_id ELSE sender_id END AS other_peer,
    sender_id AS last_sender_id,
    body,
    created_at_ms
  FROM chat_messages
  WHERE sender_id = $1 OR peer_id = $1
) t
LEFT JOIN users u ON u.id = t.other_peer
ORDER BY t.auction_id, t.other_peer, t.created_at_ms DESC`, userID)
	if err != nil {
		return []domain.ChatThread{}
	}
	defer rows.Close()
	out := make([]domain.ChatThread, 0, limit)
	for rows.Next() {
		var th domain.ChatThread
		if err := rows.Scan(
			&th.AuctionID, &th.OwnerID, &th.PeerID, &th.PeerName,
			&th.PeerPhone, &th.PeerReceiveCalls, &th.PeerGender, &th.PeerFamilyName,
			&th.PeerAvatarKey,
			&th.LastBody, &th.LastAtMs, &th.LastSenderID,
		); err != nil {
			continue
		}
		out = append(out, th)
		if len(out) >= limit {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastAtMs > out[j].LastAtMs })
	return out
}

func (r *ChatRepository) InsertMessage(auctionID string, input domain.SendChatInput) (domain.ChatMessage, bool) {
	ctx := context.Background()
	now := time.Now().UnixMilli()
	peerID := strings.TrimSpace(input.PeerID)
	body := strings.TrimSpace(input.Body)
	attachmentURL := strings.TrimSpace(input.AttachmentURL)
	if body == "" && attachmentURL == "" {
		return domain.ChatMessage{}, false
	}
	ownerID := ""
	_ = r.pool.QueryRow(ctx, `SELECT owner_id FROM auctions WHERE id = $1`, auctionID).Scan(&ownerID)
	var m domain.ChatMessage
	err := r.pool.QueryRow(ctx, `
INSERT INTO chat_messages (auction_id, auction_owner_id, sender_id, peer_id, sender_name, body, attachment_url, created_at_ms)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, auction_id, sender_id, peer_id, sender_name, body, attachment_url, created_at_ms`,
		auctionID, ownerID, input.SenderID, peerID, input.SenderName, body, attachmentURL, now,
	).Scan(
		&m.ID, &m.AuctionID, &m.SenderID, &m.PeerID, &m.SenderName,
		&m.Body, &m.AttachmentURL, &m.CreatedAtMs,
	)
	if err != nil {
		return domain.ChatMessage{}, false
	}
	return m, true
}

func (r *ChatRepository) ReportChat(input domain.ChatReportInput) bool {
	ctx := context.Background()
	now := time.Now().UnixMilli()
	_, err := r.pool.Exec(ctx, `
INSERT INTO chat_reports (auction_id, reporter_id, peer_id, body, created_at_ms)
VALUES ($1, $2, $3, $4, $5)`,
		strings.TrimSpace(input.AuctionID),
		strings.TrimSpace(input.ReporterID),
		strings.TrimSpace(input.PeerID),
		strings.TrimSpace(input.Body),
		now,
	)
	return err == nil
}

func (r *ChatRepository) ListSupportThreads(search string, limit int) []domain.AdminTicketThread {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	search = strings.TrimSpace(search)
	pattern := "%"
	if search != "" {
		pattern = "%" + strings.ToLower(search) + "%"
	}
	ctx := context.Background()
	rows, err := r.pool.Query(ctx, `
SELECT DISTINCT ON (u.id)
  u.id,
  COALESCE(NULLIF(TRIM(u.display_name), ''), u.id) AS display_name,
  COALESCE(u.phone, ''),
  CASE
    WHEN TRIM(cm.body) <> '' THEN cm.body
    WHEN TRIM(cm.attachment_url) <> '' THEN '📷 تصویر'
    ELSE ''
  END,
  cm.created_at_ms,
  cm.sender_id
FROM users u
JOIN chat_messages cm
  ON cm.auction_id = $1
 AND (cm.sender_id = u.id OR cm.peer_id = u.id)
WHERE ($2 = '%%')
   OR LOWER(u.id) LIKE $2
   OR LOWER(COALESCE(u.username, '')) LIKE $2
   OR LOWER(COALESCE(u.display_name, '')) LIKE $2
   OR LOWER(COALESCE(u.phone, '')) LIKE $2
ORDER BY u.id, cm.created_at_ms DESC
LIMIT $3`, domain.SupportAuctionID, pattern, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]domain.AdminTicketThread, 0, limit)
	for rows.Next() {
		var t domain.AdminTicketThread
		if err := rows.Scan(&t.UserID, &t.DisplayName, &t.Phone, &t.LastBody, &t.LastAtMs, &t.LastSender); err != nil {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastAtMs > out[j].LastAtMs })
	return out
}
