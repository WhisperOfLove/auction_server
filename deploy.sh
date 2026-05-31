#!/usr/bin/env bash
# Run on the VPS after copying the project with scp (from ~/auction_server).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

echo "==> auction_server deploy"

ENV_FILE=""
if [[ -f env.txt ]]; then
  ENV_FILE=env.txt
elif [[ -f .env ]]; then
  ENV_FILE=.env
fi

if [[ -n "$ENV_FILE" ]]; then
  sed -i 's/\r$//' "$ENV_FILE"
  set -a
  # shellcheck disable=SC1090
  . "./$ENV_FILE"
  set +a
  echo "    loaded: $ENV_FILE"
else
  echo "ERROR: no env.txt or .env in $ROOT"
  echo "  On PC: edit ../haraj.config.json then: python ../scripts/sync_haraj_config.py"
  exit 1
fi

if [[ "${POSTGRES_DSN:-}" == *"YOUR_PASSWORD"* ]]; then
  echo "ERROR: POSTGRES_DSN still contains YOUR_PASSWORD — fix env.txt (one DSN only)."
  exit 1
fi

DB_USER="${POSTGRES_USER:-auction}"
DB_PASS="${POSTGRES_PASSWORD:-auction}"
DB_NAME="${POSTGRES_DB:-auction}"

if [[ -z "${POSTGRES_DSN:-}" ]]; then
  export POSTGRES_DSN="postgres://${DB_USER}:${DB_PASS}@localhost:5432/${DB_NAME}?sslmode=disable"
fi

export PUBLIC_BASE_URL="${PUBLIC_BASE_URL:-http://127.0.0.1:${APP_PORT:-8080}}"
export UPLOAD_DIR="${UPLOAD_DIR:-uploads}"
export APP_PORT="${APP_PORT:-8080}"

if command -v psql >/dev/null 2>&1; then
  echo "==> PostgreSQL user/database"
  sudo -u postgres psql -v ON_ERROR_STOP=1 <<SQL
DO \$\$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${DB_USER}') THEN
    CREATE USER ${DB_USER} WITH PASSWORD '${DB_PASS}';
  ELSE
    ALTER USER ${DB_USER} WITH PASSWORD '${DB_PASS}';
  END IF;
END
\$\$;
SELECT 'CREATE DATABASE ${DB_NAME} OWNER ${DB_USER}'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '${DB_NAME}')\gexec
GRANT ALL PRIVILEGES ON DATABASE ${DB_NAME} TO ${DB_USER};
SQL
  sudo -u postgres psql -d "${DB_NAME}" -v ON_ERROR_STOP=1 <<SQL
GRANT ALL ON SCHEMA public TO ${DB_USER};
GRANT CREATE ON SCHEMA public TO ${DB_USER};
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO ${DB_USER};
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO ${DB_USER};
SQL
else
  echo "WARNING: psql not found — install: sudo apt install -y postgresql postgresql-contrib"
fi

mkdir -p "$UPLOAD_DIR"

echo "==> start API"
if [[ -f ./auction_api ]]; then
  chmod +x ./auction_api 2>/dev/null || true
  echo "    using pre-built auction_api from bundle"
elif [[ -d vendor ]] && [[ -d vendor/github.com/jackc/pgx/v5 ]] && command -v go >/dev/null 2>&1; then
  echo "    building from source (vendor/ on VPS)"
  go build -mod=vendor -o auction_api ./cmd/api
else
  echo "ERROR: no auction_api binary and cannot build (need vendor/ + go, or upload binary via deploy-fast.ps1)"
  exit 1
fi

echo "==> restart API"
pkill -f './auction_api' 2>/dev/null || true
sleep 1
nohup env \
  POSTGRES_DSN="$POSTGRES_DSN" \
  PUBLIC_BASE_URL="$PUBLIC_BASE_URL" \
  UPLOAD_DIR="$UPLOAD_DIR" \
  APP_PORT="$APP_PORT" \
  FEED_VISIBLE_HOURS="${FEED_VISIBLE_HOURS:-72}" \
  MY_AUCTIONS_DELETE_DAYS_AFTER_EXPIRY="${MY_AUCTIONS_DELETE_DAYS_AFTER_EXPIRY:-0}" \
  REDIS_ADDR="${REDIS_ADDR:-}" \
  KAFKA_BROKERS="${KAFKA_BROKERS:-}" \
  TOP_BIDS_PUSH_INTERVAL_SECONDS="${TOP_BIDS_PUSH_INTERVAL_SECONDS:-10}" \
  FEED_REFRESH_INTERVAL_SECONDS="${FEED_REFRESH_INTERVAL_SECONDS:-30}" \
  ./auction_api >> auction_api.log 2>&1 &
sleep 2

if ! curl -sf "http://127.0.0.1:${APP_PORT}/health" >/dev/null; then
  echo "ERROR: API did not respond on localhost:${APP_PORT}/health"
  echo "Last log lines:"
  tail -30 auction_api.log 2>/dev/null || true
  exit 1
fi

echo "OK localhost health:"
curl -s "http://127.0.0.1:${APP_PORT}/health"
echo ""

if command -v ss >/dev/null 2>&1; then
  echo "Listening:"
  ss -tlnp 2>/dev/null | grep ":${APP_PORT} " || ss -tln | grep ":${APP_PORT} " || true
fi

if command -v ufw >/dev/null 2>&1 && sudo ufw status 2>/dev/null | grep -q "Status: active"; then
  if ! sudo ufw status | grep -q "${APP_PORT}/tcp"; then
    echo "TIP: open firewall: sudo ufw allow ${APP_PORT}/tcp"
  fi
fi

echo "Done. Logs: tail -f auction_api.log"
echo "Public URL: ${PUBLIC_BASE_URL}/health (also open port ${APP_PORT} in Arvan cloud firewall)"
