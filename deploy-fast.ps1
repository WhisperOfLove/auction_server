# Deploy Go API to VPS (offline build via vendor/).
# On PC (auction_server folder):
#   go mod vendor
#   .\deploy-fast.ps1
#
# On VPS (one command — fresh DB + unzip + start):
#   cd ~/auction_server && chmod +x server-fresh.sh && ./server-fresh.sh

$ErrorActionPreference = "Stop"
$RemoteHost = "ubuntu@37.32.29.210"
$RemoteDir = "auction_server"
$BundleName = "deploy-bundle.tar.gz"
$Root = $PSScriptRoot
$BundlePath = Join-Path $Root $BundleName

Set-Location $Root

$SyncScript = Join-Path (Split-Path $Root -Parent) "scripts\sync_haraj_config.py"
if (Test-Path $SyncScript) {
    Write-Host "Syncing haraj.config.json -> env.txt ..."
    python $SyncScript
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}

if (-not (Test-Path "vendor\github.com\jackc\pgx\v5")) {
    Write-Host "ERROR: vendor incomplete. Run: go mod vendor" -ForegroundColor Red
    exit 1
}

Write-Host "Building Linux binary (auction_api) ..."
$env:GOOS = "linux"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
go build -mod=vendor -o auction_api ./cmd/api
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
if (-not (Test-Path "auction_api")) {
    Write-Host "ERROR: go build did not produce auction_api" -ForegroundColor Red
    exit 1
}

$items = @("cmd", "internal", "vendor", "go.mod", "go.sum", "env.txt", "deploy.sh", "status.sh", "check-vps.sh", "server-fresh.sh", "server-keep.sh", "auction_api", "docs")
foreach ($i in $items) {
    if (-not (Test-Path $i)) {
        Write-Host "ERROR: missing $i" -ForegroundColor Red
        exit 1
    }
}

Write-Host "Packing $BundleName (tar preserves vendor/ correctly) ..."
if (Test-Path $BundlePath) { Remove-Item $BundlePath -Force }
& tar -czf $BundleName @items
if ($LASTEXITCODE -ne 0) {
    Write-Host "tar failed. Install tar (Windows 10+) or use WSL." -ForegroundColor Red
    exit 1
}
$sizeMb = [math]::Round((Get-Item $BundlePath).Length / 1MB, 1)
Write-Host "Bundle size: ${sizeMb} MB"

Write-Host "Uploading ..."
ssh $RemoteHost "mkdir -p $RemoteDir"
scp $BundlePath "${RemoteHost}:${RemoteDir}/"
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host ""
Write-Host "Upload OK. On the server (SSH in):" -ForegroundColor Green
Write-Host @"

  cd ~/auction_server
  tar -xzf deploy-bundle.tar.gz
  chmod +x server-keep.sh server-fresh.sh deploy.sh status.sh check-vps.sh
  sed -i 's/\r$//' *.sh env.txt
  ./server-keep.sh          # KEEPS old posts in PostgreSQL (normal deploy)
  # ./server-fresh.sh       # WIPES database (all posts gone) — use only for clean start

  (Do not use server-keep if you want an empty database.)

"@
