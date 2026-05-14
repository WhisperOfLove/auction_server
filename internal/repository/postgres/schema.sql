-- Run automatically on server start (Migrate). Safe to re-run.

CREATE SEQUENCE IF NOT EXISTS auction_id_seq START 1 INCREMENT 1;

CREATE TABLE IF NOT EXISTS auctions (
    id TEXT PRIMARY KEY,
    owner_id TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    image_urls JSONB NOT NULL DEFAULT '[]'::jsonb,
    status TEXT NOT NULL,
    created_at_ms BIGINT NOT NULL,
    expires_at_ms BIGINT NOT NULL,
    ended_at_ms BIGINT
);

CREATE TABLE IF NOT EXISTS bids (
    id BIGSERIAL PRIMARY KEY,
    auction_id TEXT NOT NULL REFERENCES auctions (id) ON DELETE CASCADE,
    user_name TEXT NOT NULL,
    price BIGINT NOT NULL,
    created_at_ms BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS bids_auction_id_idx ON bids (auction_id);
CREATE INDEX IF NOT EXISTS auctions_status_idx ON auctions (status);
CREATE INDEX IF NOT EXISTS auctions_owner_idx ON auctions (owner_id);
