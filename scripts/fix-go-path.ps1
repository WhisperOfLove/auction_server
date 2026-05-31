# Fixes "GOPATH and GOROOT are the same directory" / Access denied on go mod vendor.
# Run once per terminal (or restart terminal / Cursor after first run).

$goRoot = "C:\Program Files\Go"
$goPath = Join-Path $env:USERPROFILE "go"
$modCache = Join-Path $goPath "pkg\mod"

New-Item -ItemType Directory -Force -Path $goPath, $modCache | Out-Null

[Environment]::SetEnvironmentVariable("GOPATH", $goPath, "User")
[Environment]::SetEnvironmentVariable("GOMODCACHE", $modCache, "User")
if ([Environment]::GetEnvironmentVariable("GOPATH", "Machine") -eq $goRoot) {
    [Environment]::SetEnvironmentVariable("GOPATH", $null, "Machine")
}
if ([Environment]::GetEnvironmentVariable("GOROOT", "User") -eq $goRoot) {
    [Environment]::SetEnvironmentVariable("GOROOT", $null, "User")
}

& go env -w "GOPATH=$goPath"
& go env -w "GOMODCACHE=$modCache"

$env:GOROOT = $goRoot
$env:GOPATH = $goPath
$env:GOMODCACHE = $modCache

Write-Host "Go paths fixed for this session and saved for new terminals."
Write-Host ""
go env GOPATH GOROOT GOMODCACHE
