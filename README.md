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
- Placeholders for Redis/Kafka/Postgres integrations
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

## Run locally now
1. Open terminal in `auction_server`
2. Run:
   - `go run ./cmd/api`
3. Test:
   - `GET http://localhost:8080/health`
   - `GET http://localhost:8080/v1/auctions`

## Next steps
1. Implement auth service (phone OTP/JWT)
2. Add websocket gateway for realtime auction detail updates
3. Persist to PostgreSQL
4. Add Redis bid engine + Kafka events
5. Connect Android `RemoteAuctionRepository` to these APIs
