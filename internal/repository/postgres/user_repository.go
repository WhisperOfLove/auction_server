package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"auction_server/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

const userSelectCols = `
id, COALESCE(user_number, 0), username, display_name, gender, account_type, phone, city, avatar_key,
receive_calls, subscription_plan,
COALESCE(subscription_buy_at_ms, 0), COALESCE(subscription_expire_at_ms, 0),
bio, verified`

func scanUser(row pgx.Row) (domain.User, error) {
	var u domain.User
	err := row.Scan(
		&u.ID, &u.UserNumber, &u.Username, &u.CustomName,
		&u.Gender, &u.AccountType, &u.Phone, &u.City, &u.AvatarKey, &u.ReceiveCalls,
		&u.SubscriptionPlan, &u.SubscriptionBuyAtMs, &u.SubscriptionExpireAtMs,
		&u.Bio, &u.Verified,
	)
	if err != nil {
		return u, err
	}
	u.Name = domain.ResolveName(u.ID, u.CustomName)
	return u, nil
}

func (r *UserRepository) Login(username, password string) (domain.User, bool) {
	ctx := context.Background()
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" || password == "" {
		return domain.User{}, false
	}
	row := r.pool.QueryRow(ctx, `
SELECT `+userSelectCols+`
FROM users
WHERE LOWER(username) = LOWER($1) AND LOWER(password) = LOWER($2)`, username, password)
	u, err := scanUser(row)
	if err != nil {
		return domain.User{}, false
	}
	return u, true
}

func (r *UserRepository) UpsertUser(input domain.UpsertUserInput) (domain.User, bool) {
	ctx := context.Background()
	id := strings.TrimSpace(input.ID)
	phone := strings.TrimSpace(input.Phone)
	if phone == "" {
		return domain.User{}, false
	}
	if id == "" {
		return r.RegisterPhone(phone)
	}
	_, err := r.pool.Exec(ctx, `
INSERT INTO users (id, phone, user_number)
VALUES ($1, $2, nextval('users_user_number_seq'))
ON CONFLICT (id) DO UPDATE SET phone = EXCLUDED.phone`,
		id, phone)
	if err != nil {
		return domain.User{}, false
	}
	return r.GetByID(id)
}

func (r *UserRepository) RegisterPhone(phone string) (domain.User, bool) {
	ctx := context.Background()
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return domain.User{}, false
	}
	row := r.pool.QueryRow(ctx, `
WITH n AS (SELECT nextval('users_user_number_seq') AS num)
INSERT INTO users (id, phone, user_number, username, password, display_name)
SELECT num::text, $1, num, '', '', '' FROM n
RETURNING `+userSelectCols, phone)
	u, err := scanUser(row)
	if err != nil {
		return domain.User{}, false
	}
	return u, true
}

func (r *UserRepository) GetByID(id string) (domain.User, bool) {
	ctx := context.Background()
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.User{}, false
	}
	row := r.pool.QueryRow(ctx, `SELECT `+userSelectCols+` FROM users WHERE id = $1`, id)
	u, err := scanUser(row)
	if err != nil {
		return domain.User{}, false
	}
	return u, true
}

var ErrDisplayNameTaken = errors.New("display name taken")

func (r *UserRepository) UpdateProfile(input domain.ProfileUpdateInput) (domain.User, error) {
	ctx := context.Background()
	input.UserID = strings.TrimSpace(input.UserID)
	if input.UserID == "" {
		return domain.User{}, errors.New("user id required")
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name != "" {
			_, err := r.pool.Exec(ctx, `UPDATE users SET display_name = $2 WHERE id = $1`, input.UserID, name)
			if err != nil {
				return domain.User{}, err
			}
		} else {
			_, _ = r.pool.Exec(ctx, `UPDATE users SET display_name = '' WHERE id = $1`, input.UserID)
		}
	}
	if input.City != "" {
		_, _ = r.pool.Exec(ctx, `UPDATE users SET city = $2 WHERE id = $1`, input.UserID, strings.TrimSpace(input.City))
	}
	if input.AvatarKey != "" {
		_, _ = r.pool.Exec(ctx, `UPDATE users SET avatar_key = $2 WHERE id = $1`, input.UserID, strings.TrimSpace(input.AvatarKey))
	}
	if input.Bio != nil {
		_, _ = r.pool.Exec(ctx, `UPDATE users SET bio = $2 WHERE id = $1`, input.UserID, strings.TrimSpace(*input.Bio))
	}
	if input.ReceiveCalls != nil {
		_, _ = r.pool.Exec(ctx, `UPDATE users SET receive_calls = $2 WHERE id = $1`, input.UserID, *input.ReceiveCalls)
	}
	u, ok := r.GetByID(input.UserID)
	if !ok {
		return domain.User{}, fmt.Errorf("user not found")
	}
	return u, nil
}

// BidAuctionIDs returns auction IDs where the user placed a bid (for cache invalidation).
func (r *UserRepository) BidAuctionIDs(userID string) []string {
	ctx := context.Background()
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	rows, err := r.pool.Query(ctx, `
SELECT DISTINCT auction_id FROM bids WHERE user_id = $1`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil && id != "" {
			out = append(out, id)
		}
	}
	return out
}

func (r *UserRepository) Follow(followerID, followedID string) bool {
	ctx := context.Background()
	followerID = strings.TrimSpace(followerID)
	followedID = strings.TrimSpace(followedID)
	if followerID == "" || followedID == "" || followerID == followedID {
		return false
	}
	_, err := r.pool.Exec(ctx, `
INSERT INTO user_follows (follower_id, followed_id, created_at_ms)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING`,
		followerID, followedID, time.Now().UnixMilli())
	return err == nil
}

func (r *UserRepository) Unfollow(followerID, followedID string) bool {
	ctx := context.Background()
	_, err := r.pool.Exec(ctx, `DELETE FROM user_follows WHERE follower_id = $1 AND followed_id = $2`,
		strings.TrimSpace(followerID), strings.TrimSpace(followedID))
	return err == nil
}

func (r *UserRepository) IsFollowing(followerID, followedID string) bool {
	ctx := context.Background()
	var n int
	err := r.pool.QueryRow(ctx, `
SELECT 1 FROM user_follows WHERE follower_id = $1 AND followed_id = $2 LIMIT 1`,
		strings.TrimSpace(followerID), strings.TrimSpace(followedID)).Scan(&n)
	return err == nil && n == 1
}

func (r *UserRepository) CountFollowers(userID string) int {
	ctx := context.Background()
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return 0
	}
	var n int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM user_follows WHERE followed_id = $1`, userID).Scan(&n)
	if err != nil {
		return 0
	}
	return n
}

func (r *UserRepository) ListFollowing(followerID string, limit int) []domain.FollowUserSummary {
	ctx := context.Background()
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
SELECT u.id, u.display_name, COALESCE(u.user_number, 0), u.phone, u.avatar_key
FROM user_follows f
JOIN users u ON u.id = f.followed_id
WHERE f.follower_id = $1
ORDER BY f.created_at_ms DESC
LIMIT $2`, strings.TrimSpace(followerID), limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []domain.FollowUserSummary
	for rows.Next() {
		var s domain.FollowUserSummary
		var u domain.User
		if rows.Scan(&s.ID, &u.CustomName, &u.UserNumber, &s.Phone, &s.AvatarKey) == nil {
			s.Name = domain.ResolveName(u.ID, u.CustomName)
			out = append(out, s)
		}
	}
	return out
}

func (r *UserRepository) Block(blockerID, blockedID string) bool {
	ctx := context.Background()
	blockerID = strings.TrimSpace(blockerID)
	blockedID = strings.TrimSpace(blockedID)
	if blockerID == "" || blockedID == "" || blockerID == blockedID {
		return false
	}
	_, err := r.pool.Exec(ctx, `
INSERT INTO user_blocks (blocker_id, blocked_id, created_at_ms)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING`,
		blockerID, blockedID, time.Now().UnixMilli())
	if err != nil {
		return false
	}
	// Block is one-way for chat; also stop following both ways.
	_, _ = r.pool.Exec(ctx, `DELETE FROM user_follows WHERE follower_id = $1 AND followed_id = $2`,
		blockerID, blockedID)
	_, _ = r.pool.Exec(ctx, `DELETE FROM user_follows WHERE follower_id = $1 AND followed_id = $2`,
		blockedID, blockerID)
	return true
}

func (r *UserRepository) Unblock(blockerID, blockedID string) bool {
	ctx := context.Background()
	_, err := r.pool.Exec(ctx, `DELETE FROM user_blocks WHERE blocker_id = $1 AND blocked_id = $2`,
		strings.TrimSpace(blockerID), strings.TrimSpace(blockedID))
	return err == nil
}

func (r *UserRepository) IsBlocked(blockerID, blockedID string) bool {
	ctx := context.Background()
	var n int
	err := r.pool.QueryRow(ctx, `
SELECT 1 FROM user_blocks WHERE blocker_id = $1 AND blocked_id = $2 LIMIT 1`,
		strings.TrimSpace(blockerID), strings.TrimSpace(blockedID)).Scan(&n)
	return err == nil && n == 1
}

func (r *UserRepository) BlockStatus(viewerID, peerID string) domain.BlockStatus {
	viewerID = strings.TrimSpace(viewerID)
	peerID = strings.TrimSpace(peerID)
	byViewer := r.IsBlocked(viewerID, peerID)
	byPeer := r.IsBlocked(peerID, viewerID)
	return domain.BlockStatus{
		BlockedByViewer: byViewer,
		BlockedByPeer:   byPeer,
		CanChat:         viewerID != "" && peerID != "" && viewerID != peerID && !byViewer && !byPeer,
	}
}

func (r *UserRepository) ListBlocked(blockerID string, limit int) []domain.BlockUserSummary {
	ctx := context.Background()
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
SELECT u.id, u.display_name, COALESCE(u.user_number, 0), u.avatar_key
FROM user_blocks b
JOIN users u ON u.id = b.blocked_id
WHERE b.blocker_id = $1
ORDER BY b.created_at_ms DESC
LIMIT $2`, strings.TrimSpace(blockerID), limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []domain.BlockUserSummary
	for rows.Next() {
		var s domain.BlockUserSummary
		var u domain.User
		if rows.Scan(&s.ID, &u.CustomName, &u.UserNumber, &s.AvatarKey) == nil {
			s.Name = domain.ResolveName(u.ID, u.CustomName)
			out = append(out, s)
		}
	}
	return out
}

func (r *UserRepository) ListForAdmin(search string, limit, offset int) domain.AdminUsersPage {
	ctx := context.Background()
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	search = strings.TrimSpace(search)
	pattern := "%"
	if search != "" {
		pattern = "%" + strings.ToLower(search) + "%"
	}
	rows, err := r.pool.Query(ctx, `
SELECT
  u.id,
  COALESCE(u.user_number, 0),
  COALESCE(u.username, ''),
  COALESCE(u.phone, ''),
  COALESCE(u.subscription_plan, ''),
  COALESCE(u.created_at_ms, 0),
  COALESCE(u.subscription_buy_at_ms, 0),
  COALESCE(u.subscription_expire_at_ms, 0),
  COALESCE(COUNT(a.id), 0)::bigint AS post_count
FROM users u
LEFT JOIN auctions a ON a.owner_id = u.id
WHERE ($1 = '%%')
   OR LOWER(u.id) LIKE $1
   OR LOWER(COALESCE(u.username, '')) LIKE $1
   OR LOWER(COALESCE(u.display_name, '')) LIKE $1
   OR LOWER(COALESCE(u.phone, '')) LIKE $1
   OR CAST(COALESCE(u.user_number, 0) AS TEXT) LIKE $1
GROUP BY u.id, u.user_number, u.username, u.phone, u.subscription_plan, u.created_at_ms, u.subscription_buy_at_ms, u.subscription_expire_at_ms
ORDER BY COALESCE(u.created_at_ms, 0) DESC, u.id DESC
LIMIT $2 OFFSET $3`, pattern, limit, offset)
	if err != nil {
		return domain.AdminUsersPage{}
	}
	defer rows.Close()
	out := make([]domain.AdminUserRow, 0, limit)
	for rows.Next() {
		var it domain.AdminUserRow
		if err := rows.Scan(
			&it.ID, &it.UserNumber, &it.Username, &it.Phone, &it.SubscriptionPlan,
			&it.RegisteredAtMs, &it.SubscriptionBuyAtMs, &it.SubscriptionExpireAtMs, &it.PostCount,
		); err != nil {
			continue
		}
		out = append(out, it)
	}
	var total int64
	_ = r.pool.QueryRow(ctx, `
SELECT COUNT(*)::bigint
FROM users u
WHERE ($1 = '%%')
   OR LOWER(u.id) LIKE $1
   OR LOWER(COALESCE(u.username, '')) LIKE $1
   OR LOWER(COALESCE(u.display_name, '')) LIKE $1
   OR LOWER(COALESCE(u.phone, '')) LIKE $1
   OR CAST(COALESCE(u.user_number, 0) AS TEXT) LIKE $1`, pattern).Scan(&total)
	return domain.AdminUsersPage{Items: out, Total: total}
}

func (r *UserRepository) RegistrationPeriodCounts() []domain.AdminPeriodCount {
	ctx := context.Background()
	now := time.Now().UnixMilli()
	day := now - 24*60*60*1000
	week := now - 7*24*60*60*1000
	month := now - 30*24*60*60*1000
	var d, w, m int64
	_ = r.pool.QueryRow(ctx, `
SELECT
  COUNT(*) FILTER (WHERE COALESCE(created_at_ms, 0) >= $1)::bigint AS day_count,
  COUNT(*) FILTER (WHERE COALESCE(created_at_ms, 0) >= $2)::bigint AS week_count,
  COUNT(*) FILTER (WHERE COALESCE(created_at_ms, 0) >= $3)::bigint AS month_count
FROM users`, day, week, month).Scan(&d, &w, &m)
	return []domain.AdminPeriodCount{
		{Label: "روز", Value: d},
		{Label: "هفته", Value: w},
		{Label: "ماه", Value: m},
	}
}
