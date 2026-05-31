#!/usr/bin/env bash
# Full VPS verification for auction_server (run on the VPS).
# Usage: cd ~/auction_server && chmod +x check-vps.sh && ./check-vps.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

APP_PORT=8080
PUBLIC_BASE=""
REDIS_ADDR=""
KAFKA_BROKERS=""

if [[ -f env.txt ]]; then
  sed -i 's/\r$//' env.txt 2>/dev/null || true
  # shellcheck disable=SC1091
  . env.txt
fi
APP_PORT="${APP_PORT:-8080}"
PUBLIC_BASE="${PUBLIC_BASE_URL:-http://127.0.0.1:${APP_PORT}}"

pass() { echo "  OK  $*"; }
fail() { echo "  FAIL $*"; }
warn() { echo "  WARN $*"; }

echo "=============================================="
echo " Haraj VPS check — $(date -Iseconds 2>/dev/null || date)"
echo "=============================================="

# --- Process ---
echo ""
echo "[1] API process"
if pgrep -af './auction_api' >/dev/null 2>&1; then
  pass "auction_api running"
  pgrep -af './auction_api' | head -1 || true
else
  fail "auction_api not running — run ./deploy.sh or ./server-keep.sh"
fi

# --- Health ---
echo ""
echo "[2] HTTP health"
if curl -sf -m 5 "http://127.0.0.1:${APP_PORT}/health" >/dev/null; then
  pass "localhost:${APP_PORT}/health"
  curl -s "http://127.0.0.1:${APP_PORT}/health"
  echo ""
else
  fail "localhost health — see auction_api.log"
fi

if [[ "$PUBLIC_BASE" != *"127.0.0.1"* ]]; then
  if curl -sf -m 8 "${PUBLIC_BASE%/}/health" >/dev/null 2>&1; then
    pass "public ${PUBLIC_BASE%/}/health"
  else
    warn "public health failed (Arvan firewall / wrong PUBLIC_BASE_URL?)"
  fi
fi

# --- PostgreSQL ---
echo ""
echo "[3] PostgreSQL"
if [[ -z "${POSTGRES_DSN:-}" ]]; then
  fail "POSTGRES_DSN not set — API uses in-memory only"
else
  pass "POSTGRES_DSN set"
  if command -v psql >/dev/null 2>&1; then
    count=$(psql "$POSTGRES_DSN" -t -A -c "SELECT COUNT(*) FROM auctions;" 2>/dev/null || echo "")
    if [[ -n "$count" ]]; then
      pass "auctions in DB: $count"
    else
      fail "psql query failed"
    fi
  else
    warn "psql not installed — skip DB count"
  fi
fi

# --- New API features ---
echo ""
echo "[4] Pagination + realtime config"
feed=$(curl -s -m 8 "http://127.0.0.1:${APP_PORT}/v1/auctions?status=ACTIVE&limit=3" || true)
if echo "$feed" | grep -q '"items"'; then
  pass "paginated feed (items + cursor)"
else
  warn "feed missing items[] — old binary or deploy not updated?"
fi

rt=$(curl -s -m 5 "http://127.0.0.1:${APP_PORT}/v1/config/realtime" || true)
if echo "$rt" | grep -q 'feedRefreshIntervalSeconds'; then
  pass "realtime config endpoint"
  echo "       $rt"
else
  warn "realtime config missing"
fi

# --- Logs: stack features ---
echo ""
echo "[5] Startup log (redis / kafka / expiry / storage)"
if [[ -f auction_api.log ]]; then
  if grep -q 'expiry job started' auction_api.log 2>/dev/null; then
    pass "expiry cron"
  else
    warn "no 'expiry job started' in log — redeploy new API?"
  fi
  if grep -q 'storage: PostgreSQL' auction_api.log 2>/dev/null; then
    pass "PostgreSQL storage"
  elif grep -q 'in-memory' auction_api.log 2>/dev/null; then
    fail "in-memory storage in log"
  fi
  if grep -q 'redis: connected' auction_api.log 2>/dev/null; then
    pass "Redis connected"
  elif grep -q 'redis: disabled' auction_api.log 2>/dev/null; then
    warn "Redis disabled (optional)"
  fi
  if grep -q 'kafka: producer ready' auction_api.log 2>/dev/null; then
    pass "Kafka producer"
  elif grep -q 'kafka: disabled' auction_api.log 2>/dev/null; then
    warn "Kafka disabled (optional)"
  fi
else
  warn "no auction_api.log"
fi

# --- Optional Redis ---
echo ""
echo "[6] Redis (optional)"
if [[ -n "${REDIS_ADDR:-}" ]]; then
  if command -v redis-cli >/dev/null 2>&1; then
    host="${REDIS_ADDR%%:*}"
    port="${REDIS_ADDR##*:}"
    if redis-cli -h "${host:-127.0.0.1}" -p "${port:-6379}" ping 2>/dev/null | grep -q PONG; then
      pass "redis-cli ping ($REDIS_ADDR)"
    else
      fail "REDIS_ADDR set but redis not responding"
    fi
  else
    warn "REDIS_ADDR=$REDIS_ADDR but redis-cli not installed"
  fi
else
  warn "REDIS_ADDR empty — cache off (OK)"
fi

# --- Optional Nginx ---
echo ""
echo "[7] Nginx (optional)"
if command -v nginx >/dev/null 2>&1 && systemctl is-active nginx >/dev/null 2>&1; then
  pass "nginx active"
else
  warn "nginx not active — API on :${APP_PORT} direct (OK)"
fi

# --- Port listen ---
echo ""
echo "[8] Listening port"
if command -v ss >/dev/null 2>&1; then
  ss -tln | grep ":${APP_PORT} " && pass "port ${APP_PORT} listening" || fail "port ${APP_PORT} not listening"
fi

echo ""
echo "=============================================="
echo " Done. See docs/VPS_CHECKLIST.md for fixes."
echo "=============================================="
