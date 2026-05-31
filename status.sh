#!/usr/bin/env bash
# Quick check on the VPS: cd ~/auction_server && chmod +x status.sh && ./status.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

APP_PORT=8080
if [[ -f env.txt ]]; then
  sed -i 's/\r$//' env.txt 2>/dev/null || true
  # shellcheck disable=SC1091
  . env.txt
fi
APP_PORT="${APP_PORT:-8080}"

echo "=== auction_server status ==="
echo "PWD: $ROOT"

if pgrep -af './auction_api' >/dev/null 2>&1; then
  echo "Process: RUNNING"
  pgrep -af './auction_api' || true
else
  echo "Process: NOT RUNNING"
fi

echo ""
echo "--- localhost:${APP_PORT}/health ---"
curl -sS -m 3 "http://127.0.0.1:${APP_PORT}/health" 2>&1 || echo "(failed)"

echo ""
echo "--- POSTGRES_DSN (masked) ---"
if [[ -n "${POSTGRES_DSN:-}" ]]; then
  echo "${POSTGRES_DSN/@*@/@***@}"
else
  echo "(not set — API would use in-memory only)"
fi

if command -v psql >/dev/null 2>&1 && [[ -n "${POSTGRES_DSN:-}" ]]; then
  echo ""
  echo "--- auctions count ---"
  psql "$POSTGRES_DSN" -t -c "SELECT COUNT(*) FROM auctions;" 2>&1 || echo "(psql failed)"
fi

echo ""
echo "--- last 15 log lines ---"
tail -15 auction_api.log 2>/dev/null || echo "(no auction_api.log)"
