#!/usr/bin/env bash
# One command on VPS after uploading deploy-bundle.tar.gz (or .zip):
#   cd ~/auction_server && chmod +x server-fresh.sh && ./server-fresh.sh
set -euo pipefail
cd "$(dirname "$0")"

echo "==> stop API"
pkill -f './auction_api' 2>/dev/null || true
sleep 1

echo "==> unpack bundle (if present)"
if [[ -f deploy-bundle.tar.gz ]]; then
  tar -xzf deploy-bundle.tar.gz
elif [[ -f deploy-bundle.zip ]]; then
  if ! command -v unzip >/dev/null 2>&1; then
    sudo DEBIAN_FRONTEND=noninteractive apt-get update -qq
    sudo DEBIAN_FRONTEND=noninteractive apt-get install -y unzip
  fi
  unzip -o -q deploy-bundle.zip
else
  echo "    (no bundle file — using files already in this folder)"
fi

echo "==> fresh PostgreSQL (drops ALL posts, bids, chat — cannot undo)"
if ! id postgres &>/dev/null; then
  echo "ERROR: PostgreSQL is not installed (system user 'postgres' missing)."
  echo "  Run on this VPS:"
  echo "    sudo apt update"
  echo "    sudo apt install -y postgresql postgresql-contrib"
  echo "    sudo systemctl enable --now postgresql"
  echo "    sudo -u postgres psql -c \"SELECT version();\""
  echo "  Then run ./server-fresh.sh again."
  exit 1
fi
if ! command -v psql &>/dev/null; then
  echo "ERROR: psql not found. Install: sudo apt install -y postgresql-client postgresql"
  exit 1
fi
if [[ -f env.txt ]]; then
  sed -i 's/\r$//' env.txt 2>/dev/null || true
  # shellcheck disable=SC1091
  . ./env.txt
fi
DB_NAME="${POSTGRES_DB:-auction}"
DB_USER="${POSTGRES_USER:-auction}"

if sudo -u postgres psql -t -A -q -c "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" 2>/dev/null | grep -q '^1$'; then
  echo "    wiping existing database '${DB_NAME}'..."
  sudo -u postgres psql -v ON_ERROR_STOP=1 <<SQL
SELECT pg_terminate_backend(pid)
FROM pg_stat_activity
WHERE datname = '${DB_NAME}' AND pid <> pg_backend_pid();
DROP DATABASE IF EXISTS ${DB_NAME};
DROP USER IF EXISTS ${DB_USER};
SQL
  echo "    dropped database: ${DB_NAME}"
else
  echo "    first deploy — database '${DB_NAME}' not found, skipping wipe"
fi

sed -i 's/\r$//' deploy.sh status.sh check-vps.sh env.txt server-fresh.sh 2>/dev/null || true
chmod +x deploy.sh status.sh check-vps.sh server-fresh.sh 2>/dev/null || true

if [[ ! -f vendor/modules.txt ]] || [[ ! -d vendor/github.com/jackc/pgx/v5 ]]; then
  echo "ERROR: vendor/ is incomplete (pgx missing)."
  echo "On your PC:  go mod vendor"
  echo "Then:        .\\deploy-fast.ps1   (uses tar.gz — fixes vendor)"
  exit 1
fi

./deploy.sh
./status.sh

if command -v psql >/dev/null 2>&1 && [[ -n "${POSTGRES_DSN:-}" ]]; then
  echo ""
  echo "==> verify DB"
  count=$(psql "$POSTGRES_DSN" -t -A -c "SELECT COUNT(*) FROM auctions;" 2>/dev/null || echo "?")
  if [[ "${count}" == "?" ]]; then
    echo "    (tables not ready yet — API will create them on first start)"
  else
    echo "    auctions in database: ${count}"
  fi
fi

echo ""
./check-vps.sh
