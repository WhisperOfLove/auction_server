# Architecture Notes

## Target scale
- 1M users
- up to 1M new auction posts/day
- realtime bids + chat

## Recommended server shape
- API Gateway / HTTP API service
- Realtime Gateway (WebSocket)
- Auction Core service
- Chat service
- Notification worker

## Data and messaging
- PostgreSQL: source of truth
- Redis: hot state + fast counters + lock/atomic bid checks
- Kafka: durable event stream for async workloads

## Realtime update policy
- Active auction detail: websocket push (seconds)
- Feed screen: paginated API + optional lightweight refresh
- Intervals controlled via config/env (no hardcoded UI constants)

## Android migration later
1. Keep current local mode while backend matures
2. Build `RemoteAuctionRepository` endpoints
3. Feature flag switch to remote for selected flows
4. Remove hardcoded sample sources after full parity
