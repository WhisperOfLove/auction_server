# Architecture Notes

## Target scale
- 1M users
- up to 1M new auction posts/day
- realtime bids + chat

## Current stack (implemented)
| Layer | Role |
|-------|------|
| **Nginx** | `deploy/nginx/haraj-api.conf` — TLS, gzip, WebSocket upgrade to Go |
| **PostgreSQL** | Source of truth; pool max 64; feed indexes in `schema.sql`; optional `deploy/postgres/tuning.sql` |
| **Expiry job** | `internal/jobs/expiry.go` — runs every minute; no longer on every HTTP read |
| **Feed API** | `GET /v1/auctions?limit=&cursor=` → `{ items, nextCursor, hasMore }` |
| **Redis** | Optional (`REDIS_ADDR`) — top-bids cache + feed page cache (15–30s TTL) |
| **WebSocket** | `GET /v1/ws/auctions/{id}/bids` — push `top_bids_updated`; Android uses OkHttp WS + slower HTTP fallback poll |
| **Kafka** | Optional (`KAFKA_BROKERS`) — `auction.bid.placed`; worker: `go run ./cmd/worker` |

## Env (VPS)
```
POSTGRES_DSN=...
PUBLIC_BASE_URL=http://95.38.186.24:8080
REDIS_ADDR=127.0.0.1:6379        # optional
KAFKA_BROKERS=127.0.0.1:9092     # optional
MY_AUCTIONS_DELETE_DAYS_AFTER_EXPIRY=0
```

After pulling: `go mod vendor` (writable GOPATH) then `go build -mod=vendor -o api ./cmd/api`.

## Android
- Feed: paginated fetch in `RemoteAuctionRepository`
- Bids: WebSocket push + 30s fallback poll (`REMOTE_TOP_BIDS_POLL_MS`)

## Later
- Chat WebSocket
- Redis atomic bid checks under very high concurrency
- FCM in worker from Kafka events
