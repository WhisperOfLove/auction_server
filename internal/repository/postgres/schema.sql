-- Run automatically on server start (Migrate). Safe to re-run.

CREATE SEQUENCE IF NOT EXISTS auction_id_seq START 1 INCREMENT 1;

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL DEFAULT '',
    password TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    subscription_plan TEXT NOT NULL DEFAULT '',
    subscription_buy_at_ms BIGINT,
    subscription_expire_at_ms BIGINT,
    created_at_ms BIGINT NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW()) * 1000)::bigint
);

CREATE TABLE IF NOT EXISTS auctions (
    id TEXT PRIMARY KEY,
    owner_id TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    image_urls JSONB NOT NULL DEFAULT '[]'::jsonb,
    status TEXT NOT NULL,
    created_at_ms BIGINT NOT NULL,
    expires_at_ms BIGINT NOT NULL,
    ended_at_ms BIGINT,
    min_bid_step BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS bids (
    id BIGSERIAL PRIMARY KEY,
    auction_id TEXT NOT NULL REFERENCES auctions (id) ON DELETE CASCADE,
    user_id TEXT NOT NULL DEFAULT '',
    user_name TEXT NOT NULL,
    phone TEXT NOT NULL DEFAULT '',
    price BIGINT NOT NULL,
    created_at_ms BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS chat_messages (
    id BIGSERIAL PRIMARY KEY,
    auction_id TEXT NOT NULL,
    auction_owner_id TEXT NOT NULL DEFAULT '',
    sender_id TEXT NOT NULL,
    peer_id TEXT NOT NULL DEFAULT '',
    sender_name TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    attachment_url TEXT NOT NULL DEFAULT '',
    created_at_ms BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS chat_reports (
    id BIGSERIAL PRIMARY KEY,
    auction_id TEXT NOT NULL,
    reporter_id TEXT NOT NULL,
    peer_id TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    created_at_ms BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS bids_auction_id_idx ON bids (auction_id);
-- One bid row per user per auction (required for INSERT ... ON CONFLICT upsert).
DROP INDEX IF EXISTS bids_auction_user_uidx;
CREATE UNIQUE INDEX IF NOT EXISTS bids_auction_user_uidx ON bids (auction_id, user_id);
ALTER TABLE auctions ADD COLUMN IF NOT EXISTS min_bid_step BIGINT NOT NULL DEFAULT 0;
ALTER TABLE auctions ADD COLUMN IF NOT EXISTS base_price BIGINT NOT NULL DEFAULT 0;
ALTER TABLE auctions ADD COLUMN IF NOT EXISTS bumped_at_ms BIGINT NOT NULL DEFAULT 0;
ALTER TABLE auctions ADD COLUMN IF NOT EXISTS view_count INT NOT NULL DEFAULT 0;
ALTER TABLE auctions ADD COLUMN IF NOT EXISTS is_featured BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS app_settings (
    id INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    new_post_enabled BOOLEAN NOT NULL DEFAULT true,
    messages_enabled BOOLEAN NOT NULL DEFAULT true,
    public_base_url TEXT NOT NULL DEFAULT '',
    updated_at_ms BIGINT NOT NULL DEFAULT 0
);
INSERT INTO app_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING;
ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS bid_cooldown_seconds INT NOT NULL DEFAULT 300;
ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS post_duration_hours INT NOT NULL DEFAULT 72;
ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS last_moment_hours INT NOT NULL DEFAULT 3;
ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS extend_duration_hours INT NOT NULL DEFAULT 72;
ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS top_bids_poll_seconds INT NOT NULL DEFAULT 30;
ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS chat_poll_seconds INT NOT NULL DEFAULT 30;
ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS feed_page_size INT NOT NULL DEFAULT 30;
ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS offline_message TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS auctions_status_idx ON auctions (status);
CREATE INDEX IF NOT EXISTS auctions_owner_idx ON auctions (owner_id);
CREATE INDEX IF NOT EXISTS auctions_created_at_idx ON auctions (created_at_ms);
CREATE INDEX IF NOT EXISTS auctions_expires_at_idx ON auctions (expires_at_ms);
CREATE INDEX IF NOT EXISTS auctions_active_feed_idx ON auctions (status, expires_at_ms DESC)
    WHERE status = 'ACTIVE';
CREATE INDEX IF NOT EXISTS bids_auction_price_idx ON bids (auction_id, price DESC, created_at_ms ASC);

-- Cooldown survives bid cancel/delete (anti-cheat).
CREATE TABLE IF NOT EXISTS bid_cooldowns (
    auction_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    last_bid_at_ms BIGINT NOT NULL,
    PRIMARY KEY (auction_id, user_id)
);
CREATE INDEX IF NOT EXISTS chat_messages_auction_id_idx ON chat_messages (auction_id);

-- upgrades for DBs created before phone column
ALTER TABLE bids ADD COLUMN IF NOT EXISTS phone TEXT NOT NULL DEFAULT '';
ALTER TABLE bids ADD COLUMN IF NOT EXISTS user_id TEXT NOT NULL DEFAULT '';
ALTER TABLE chat_messages ADD COLUMN IF NOT EXISTS peer_id TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS username TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS password TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS display_name TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS users_username_idx ON users (username) WHERE username <> '';

ALTER TABLE users ADD COLUMN IF NOT EXISTS first_name TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_name TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS gender TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS account_type TEXT NOT NULL DEFAULT 'شخصی';
ALTER TABLE chat_messages ADD COLUMN IF NOT EXISTS auction_owner_id TEXT NOT NULL DEFAULT '';
ALTER TABLE chat_messages ADD COLUMN IF NOT EXISTS attachment_url TEXT NOT NULL DEFAULT '';
ALTER TABLE chat_messages DROP CONSTRAINT IF EXISTS chat_messages_auction_id_fkey;
UPDATE chat_messages cm SET auction_owner_id = a.owner_id
FROM auctions a WHERE cm.auction_id = a.id AND cm.auction_owner_id = '';

ALTER TABLE users ADD COLUMN IF NOT EXISTS receive_calls BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS city TEXT NOT NULL DEFAULT 'تهران';
ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_key TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS bio TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS verified BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS created_at_ms BIGINT NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW()) * 1000)::bigint;

CREATE SEQUENCE IF NOT EXISTS users_user_number_seq START 10001;
ALTER TABLE users ADD COLUMN IF NOT EXISTS user_number BIGINT UNIQUE;
UPDATE users SET user_number = nextval('users_user_number_seq') WHERE user_number IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS users_display_name_unique_idx
    ON users (LOWER(TRIM(display_name)))
    WHERE TRIM(display_name) <> '';

CREATE TABLE IF NOT EXISTS user_follows (
    follower_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    followed_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at_ms BIGINT NOT NULL,
    PRIMARY KEY (follower_id, followed_id)
);
CREATE INDEX IF NOT EXISTS user_follows_follower_idx ON user_follows (follower_id);

CREATE TABLE IF NOT EXISTS user_blocks (
    blocker_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    blocked_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at_ms BIGINT NOT NULL,
    PRIMARY KEY (blocker_id, blocked_id)
);
CREATE INDEX IF NOT EXISTS user_blocks_blocker_idx ON user_blocks (blocker_id);
CREATE INDEX IF NOT EXISTS user_blocks_blocked_idx ON user_blocks (blocked_id);

-- Post moderation: new auctions start PENDING until admin approves.
ALTER TABLE auctions ADD COLUMN IF NOT EXISTS moderation_status TEXT NOT NULL DEFAULT 'APPROVED';
ALTER TABLE auctions ADD COLUMN IF NOT EXISTS moderation_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE auctions ADD COLUMN IF NOT EXISTS moderated_at_ms BIGINT;
ALTER TABLE auctions ADD COLUMN IF NOT EXISTS duration_hours INT NOT NULL DEFAULT 72;
CREATE INDEX IF NOT EXISTS auctions_moderation_pending_idx ON auctions (created_at_ms DESC)
    WHERE moderation_status = 'PENDING';

CREATE TABLE IF NOT EXISTS user_notifications (
    id BIGSERIAL PRIMARY KEY,
    user_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    title TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    ref_auction_id TEXT NOT NULL DEFAULT '',
    read_at_ms BIGINT,
    created_at_ms BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS user_notifications_user_idx ON user_notifications (user_id, created_at_ms DESC);

CREATE TABLE IF NOT EXISTS app_settings (
    id INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    new_post_enabled BOOLEAN NOT NULL DEFAULT true,
    messages_enabled BOOLEAN NOT NULL DEFAULT true,
    public_base_url TEXT NOT NULL DEFAULT '',
    updated_at_ms BIGINT NOT NULL DEFAULT 0
);
INSERT INTO app_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING;
