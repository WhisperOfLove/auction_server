# VPS checklist — `95.38.186.24`

Use this after deploy or when something breaks. Run checks **on the VPS** unless noted as **from your PC**.

Config source: edit `haraj.config.json` → `python scripts/sync_haraj_config.py` → deploy.

---

## Quick status (on VPS)

```bash
cd ~/auction_server
chmod +x status.sh check-vps.sh deploy.sh
./status.sh          # process, health, DB count, logs
./check-vps.sh       # full stack (API, pagination, WS path, optional Redis/Kafka/Nginx)
```

---

## 1. Deploy (from PC)

```powershell
cd c:\Users\Ben\OneDrive\Desktop\py
python scripts\sync_haraj_config.py
cd auction_server
go mod vendor
.\deploy-fast.ps1
```

On VPS (after `deploy-fast.ps1` uploaded the bundle):

```bash
cd ~/auction_server
tar -xzf deploy-bundle.tar.gz
chmod +x server-keep.sh server-fresh.sh deploy.sh status.sh check-vps.sh
sed -i 's/\r$//' *.sh env.txt
./server-keep.sh
# OR ./server-fresh.sh   # wipes DB — only if you want a clean start
```

`./server-keep.sh` runs `deploy.sh`, then `status.sh`, then **`check-vps.sh`** (checklist).

**Old posts still there?** You used **keep** — that is intentional. To wipe everything use **`./server-fresh.sh`** (not `server-keep.sh`). See below.

If you uploaded `.zip` instead of `.tar.gz`:

```bash
unzip -o deploy-bundle.zip
```

Fresh DB (delete all posts/users data in PostgreSQL):

```bash
cd ~/auction_server
tar -xzf deploy-bundle.tar.gz
chmod +x server-fresh.sh deploy.sh status.sh check-vps.sh
sed -i 's/\r$//' *.sh env.txt
./server-fresh.sh
```

Last line must be **`./server-fresh.sh`** — not `server-keep.sh`. After success, `check-vps` should show `auctions in DB: 0`.

Clear Redis cache after fresh DB (optional):

```bash
redis-cli FLUSHDB
```

---

## 2. Must pass (core API)

| Check | Command | OK if |
|-------|---------|--------|
| Process running | `pgrep -af './auction_api'` | one process |
| Health (local) | `curl -s http://127.0.0.1:8080/health` | `{"status":"ok",...}` |
| Health (public) | `curl -s http://95.38.186.24:8080/health` | same (from PC) |
| PostgreSQL | `grep POSTGRES_DSN env.txt` | DSN set, no `YOUR_PASSWORD` |
| DB reachable | `psql "$POSTGRES_DSN" -c 'SELECT COUNT(*) FROM auctions;'` | number, no error |
| Paginated feed | `curl -s 'http://127.0.0.1:8080/v1/auctions?status=ACTIVE&limit=5'` | JSON with `items`, `hasMore` |
| Realtime config | `curl -s http://127.0.0.1:8080/v1/config/realtime` | `feedRefreshIntervalSeconds`, `wsBaseUrl` |
| Expiry job | `grep -i 'expiry job' auction_api.log` | line present after start |
| Storage mode | `grep -i 'storage:' auction_api.log \| tail -1` | `PostgreSQL`, not `in-memory` |

---

## 3. Firewall / Arvan

| Check | Action |
|-------|--------|
| UFW | `sudo ufw allow 8080/tcp` (if UFW active) |
| Arvan cloud firewall | Allow **TCP 8080** inbound to the VPS |
| Nginx (later) | Open **80/443** instead of exposing 8080 publicly |

From PC:

```powershell
curl http://95.38.186.24:8080/health
```

If local works but PC fails → firewall/security group, not Go.

---

## 4. Optional stack

### Redis (cache)

```bash
sudo apt install -y redis-server
sudo systemctl enable redis-server
redis-cli ping   # PONG
```

In `haraj.config.json`: `"redisAddr": "localhost:6379"` → sync → redeploy.

Log after restart: `redis: connected` (or `disabled` if `REDIS_ADDR` empty).

### Kafka + worker (notifications later)

```bash
# Example with Docker (simplest on a small VPS):
docker run -d --name kafka -p 9092:9092 apache/kafka:latest
```

Set `"kafkaBrokers": "localhost:9092"` in config → sync → redeploy API.

Worker (separate terminal / systemd):

```bash
cd ~/auction_server
export KAFKA_BROKERS=localhost:9092
go run -mod=vendor ./cmd/worker
```

Log on bid: `worker event: {...}` when someone places a bid.

### Nginx (reverse proxy + WebSocket)

```bash
sudo apt install -y nginx
sudo cp deploy/nginx/haraj-api.conf /etc/nginx/sites-available/haraj-api
# Edit server_name and upstream port
sudo ln -sf /etc/nginx/sites-available/haraj-api /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
```

Update `publicBaseUrl` in `haraj.config.json` to your nginx URL, sync, rebuild app.

---

## 5. WebSocket smoke test

Replace `AUCTION_ID` with a real id from the feed.

```bash
# Top bids (ad detail):
websocat -v ws://127.0.0.1:8080/v1/ws/auctions/AUCTION_ID/bids

# Chat thread (viewer + peer ids from app):
websocat -v 'ws://127.0.0.1:8080/v1/ws/chat/threads/AUCTION_ID/VIEWER_ID/PEER_ID'

# Chat inbox:
websocat -v 'ws://127.0.0.1:8080/v1/ws/chat/inbox?userId=USER_ID'
```

Place a bid → `top_bids_updated`. Send a chat message → `chat_messages_updated` on thread sockets and `inbox_updated` on both users' inbox sockets.

Without websocat: app detail screen + log line `ws write` on server when clients connected.

---

## 6. Bid failed (“invalid bid or auction”)

**Watch logs while you tap ثبت پیشنهاد on the phone:**

```bash
cd ~/auction_server
tail -f auction_api.log
```

Look for lines starting with `[bid]`:

| Log | Meaning |
|-----|---------|
| `auction missing` | Wrong auction id or post deleted |
| `ended` / `status=ENDED` | Auction finished |
| `expired` | Past `expires_at_ms` |
| `too low` … `need>=` | Price below minimum (see number in log) |
| `insert failed` | DB error (paste line to debug) |

**Check one auction in DB** (replace `auction-1` with id from the app):

```bash
source env.txt 2>/dev/null || . ./env.txt
psql "$POSTGRES_DSN" -c "
SELECT id, status, base_price, min_bid_step, expires_at_ms,
  (SELECT MAX(price) FROM bids WHERE auction_id = a.id) AS top_bid
FROM auctions a WHERE id = 'auction-1';"
```

**Test bid from server** (replace id, user, price):

```bash
curl -s -X POST "http://127.0.0.1:8080/v1/auctions/auction-1/bids" \
  -H "Content-Type: application/json" \
  -d '{"userId":"user-behnam","userName":"test","phone":"09361207235","price":5000000}'
```

- First bid with `base_price` → `price` must be **≥ base_price** (قیمت پایه).
- After another bid exists → `price` must be **≥ top + min_bid_step**.

Redeploy after code fixes: PC `deploy-fast.ps1` → VPS `./server-keep.sh`.

---

## 7. Log triage

```bash
tail -f ~/auction_server/auction_api.log
```

| Log line | Meaning |
|----------|---------|
| `storage: PostgreSQL` | DB OK |
| `storage: in-memory` | **POSTGRES_DSN missing** — data not persisted |
| `expiry job started` | Background expiry OK |
| `redis: connected` | Cache on |
| `redis: disabled` | OK without Redis |
| `kafka: producer ready` | Events on |
| `postgres connect` / `migrate` fatal | Fix DSN or Postgres service |

---

## 7. Android aligned with server

1. `python scripts/sync_haraj_config.py`
2. Confirm `HarajConfig.REMOTE_BASE_URL` = `publicBaseUrl` in JSON
3. Rebuild / install APK
4. App online → feed loads; open ad → bids update (WS or 30s poll)

---

## One-page “am I production-ready?”

| Layer | Ready when |
|-------|------------|
| API + PG | `./status.sh` health OK + auction count works |
| Pagination | feed `?limit=5` returns `items` |
| Expiry cron | log shows expiry job |
| Redis | optional; `PONG` + log `redis: connected` |
| WebSocket | WS path reachable; app gets bid updates |
| Kafka | optional; worker receives bid events |
| Nginx | optional; public URL via 80/443 |
| Config | only `haraj.config.json` edited, then sync |
