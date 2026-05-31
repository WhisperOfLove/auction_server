#!/usr/bin/env bash
# Update code from bundle, keep PostgreSQL data.
# Usage: cd ~/auction_server && ./server-keep.sh
set -euo pipefail
cd "$(dirname "$0")"

pkill -f './auction_api' 2>/dev/null || true
sleep 1

if [[ -f deploy-bundle.tar.gz ]]; then
  tar -xzf deploy-bundle.tar.gz
elif [[ -f deploy-bundle.zip ]]; then
  command -v unzip >/dev/null 2>&1 || sudo DEBIAN_FRONTEND=noninteractive apt-get install -y unzip
  unzip -o -q deploy-bundle.zip
fi

sed -i 's/\r$//' deploy.sh status.sh check-vps.sh env.txt server-keep.sh server-fresh.sh 2>/dev/null || true
chmod +x deploy.sh status.sh check-vps.sh server-keep.sh server-fresh.sh 2>/dev/null || true

if [[ ! -f ./auction_api ]] && [[ ! -d vendor/github.com/jackc/pgx/v5 ]]; then
  echo "ERROR: no auction_api binary and no vendor/. Re-run on PC: go mod vendor && .\\deploy-fast.ps1"
  exit 1
fi

./deploy.sh
./status.sh
echo ""
./check-vps.sh
