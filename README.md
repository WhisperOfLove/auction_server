# Auction Server (Scalable Starter)

This folder is a separate backend starter for your auction app.

## Why separate?
- Keeps Android app local mode unchanged for development.
- Lets you build server and deploy independently.
- Later, app can switch from `LOCAL` to `REMOTE` data mode.

## Language choice
This starter uses **Go** for high concurrency and low latency.

Why Go for your use case:
- Great performance for websocket/chat/realtime bids
- Easy horizontal scaling
- Simple deployment (single binary)
- Clean and modular code style

## Redis / Kafka decision
For your scale target, best architecture is:
- **Redis**: hot realtime state (top bids, presence, short cache, fast atomic updates)
- **Kafka**: durable event stream (bid accepted, auction closed, notifications, analytics)

They are not replacements; they solve different problems.

## Current scope in this folder
- Clean architecture starter
- Config-driven intervals (so admin can change without code edits)
- Health endpoint
- **PostgreSQL**: set `POSTGRES_DSN` for real persistence (tables are created on startup). If unset, the server uses **in-memory** storage (handy for local runs).
- Placeholders for Redis/Kafka (not wired yet)
- Versioned API starter:
  - `GET /v1/auctions?status=ACTIVE&sort=new|trending`
  - `POST /v1/auctions`
  - `GET /v1/auctions/{id}`
  - `GET /v1/auctions/{id}/top-bids`
  - `POST /v1/auctions/{id}/bids`
  - `GET /v1/me/auctions?ownerId=user-1&status=ACTIVE|ENDED`
  - `GET /v1/me/auctions/{id}/result-contacts?ownerId=user-1`

## Implemented business flow
- Auctions are created with `ownerId`, `status`, and `expiresAtMs`.
- Expired ACTIVE auctions automatically move to ENDED state.
- Home/Trend tabs can use `GET /v1/auctions` with status/sort filters.
- Profile `حراجی‌های من` can use `GET /v1/me/auctions`.
- Owner-only ended contacts are exposed by `/result-contacts`.

## Run locally (no database)
1. Open terminal in `auction_server`
2. Run `go run ./cmd/api` (leave `POSTGRES_DSN` unset for in-memory mode)
3. Test `GET http://localhost:8080/health` and `GET http://localhost:8080/v1/auctions`

## PostgreSQL on Ubuntu (VPS)

```bash
sudo apt update
sudo apt install -y postgresql postgresql-contrib
sudo -u postgres psql <<'SQL'
CREATE USER auction WITH PASSWORD 'auction';
CREATE DATABASE auction OWNER auction;
GRANT ALL PRIVILEGES ON DATABASE auction TO auction;
\c auction
GRANT ALL ON SCHEMA public TO auction;
GRANT CREATE ON SCHEMA public TO auction;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO auction;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO auction;
SQL
```

Set on the server (e.g. in `.env`, then `set -a && source .env && set +a` before `./auction_api`):

```bash
POSTGRES_DSN=postgres://auction:auction@localhost:5432/auction?sslmode=disable
```

The API creates `auctions`, `bids`, and the id sequence on startup. Use a strong DB password in production and keep PostgreSQL bound to localhost (Ubuntu default).

## Next steps
1. Implement auth service (phone OTP/JWT)
2. Add websocket gateway for realtime auction detail updates
3. Add Redis bid engine + Kafka events
4. Connect Android `RemoteAuctionRepository` to these APIs (REMOTE mode + `REMOTE_BASE_URL`)
