package postgres

import (
	"context"
	"log"
	"strings"
	"time"

	"auction_server/internal/appsettings"

	"github.com/jackc/pgx/v5/pgxpool"
)

var _ appsettings.Provider = (*AppSettingsRepository)(nil)

type AppSettingsRepository struct {
	pool     *pgxpool.Pool
	fallback string
	defaults appsettings.Settings
}

func NewAppSettingsRepository(pool *pgxpool.Pool, defaults appsettings.Settings) *AppSettingsRepository {
	r := &AppSettingsRepository{
		pool:     pool,
		fallback: strings.TrimSpace(defaults.PublicBaseURL),
		defaults: defaults,
	}
	r.ensureRow()
	return r
}

func (r *AppSettingsRepository) ensureRow() {
	ctx := context.Background()
	d := r.defaults
	_, err := r.pool.Exec(ctx, `
INSERT INTO app_settings (
  id, new_post_enabled, messages_enabled, public_base_url, updated_at_ms,
  bid_cooldown_seconds, post_duration_hours, last_moment_hours, extend_duration_hours,
  top_bids_poll_seconds, chat_poll_seconds, feed_page_size, offline_message
)
VALUES (1, true, true, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (id) DO NOTHING`,
		r.fallback, time.Now().UnixMilli(),
		d.BidCooldownSeconds, d.PostDurationHours, d.LastMomentHours, d.ExtendDurationHours,
		d.TopBidsPollIntervalSeconds, d.ChatPollIntervalSeconds, d.FeedPageSize, d.OfflineMessage)
	if err != nil {
		log.Printf("app_settings_ensure_row_error err=%v", err)
	}
}

func (r *AppSettingsRepository) Get() appsettings.Settings {
	ctx := context.Background()
	var s appsettings.Settings
	err := r.pool.QueryRow(ctx, `
SELECT new_post_enabled, messages_enabled, COALESCE(public_base_url, ''),
  bid_cooldown_seconds, post_duration_hours, last_moment_hours, extend_duration_hours,
  top_bids_poll_seconds, chat_poll_seconds, feed_page_size, COALESCE(offline_message, '')
FROM app_settings WHERE id = 1`).Scan(
		&s.NewPostEnabled, &s.MessagesEnabled, &s.PublicBaseURL,
		&s.BidCooldownSeconds, &s.PostDurationHours, &s.LastMomentHours, &s.ExtendDurationHours,
		&s.TopBidsPollIntervalSeconds, &s.ChatPollIntervalSeconds, &s.FeedPageSize, &s.OfflineMessage,
	)
	if err != nil {
		log.Printf("app_settings_get_error err=%v", err)
		return r.defaults
	}
	return s
}

func (r *AppSettingsRepository) Apply(patch appsettings.Settings) (appsettings.Settings, error) {
	ctx := context.Background()
	now := time.Now().UnixMilli()
	_, err := r.pool.Exec(ctx, `
UPDATE app_settings SET
  new_post_enabled = $1,
  messages_enabled = $2,
  public_base_url = CASE WHEN $3 <> '' THEN $3 ELSE public_base_url END,
  bid_cooldown_seconds = $4,
  post_duration_hours = $5,
  last_moment_hours = $6,
  extend_duration_hours = $7,
  top_bids_poll_seconds = $8,
  chat_poll_seconds = $9,
  feed_page_size = $10,
  offline_message = $11,
  updated_at_ms = $12
WHERE id = 1`,
		patch.NewPostEnabled, patch.MessagesEnabled, strings.TrimSpace(patch.PublicBaseURL),
		patch.BidCooldownSeconds, patch.PostDurationHours, patch.LastMomentHours, patch.ExtendDurationHours,
		patch.TopBidsPollIntervalSeconds, patch.ChatPollIntervalSeconds, patch.FeedPageSize,
		strings.TrimSpace(patch.OfflineMessage), now)
	if err != nil {
		log.Printf("app_settings_apply_error err=%v", err)
		return patch, err
	}
	return r.Get(), nil
}

func (r *AppSettingsRepository) EffectiveBaseURL(fallback string) string {
	s := r.Get()
	if u := strings.TrimSpace(s.PublicBaseURL); u != "" {
		return strings.TrimSuffix(u, "/")
	}
	return strings.TrimSuffix(strings.TrimSpace(fallback), "/")
}
