# OrangeOJ dev script: start single Go backend (:8080) + single frontend Vite app (:5175)
# + optional judge-runtime (:9090 dev token).
# Usage: .\scripts\dev.ps1
# NOTE: kept ASCII-only on purpose - PowerShell 5.1 misparses BOM-less UTF-8 scripts.
# npm.cmd is used explicitly because Start-Process "npm" resolves to the
# extension-less bash shim, which is not a Win32 executable.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

Write-Host "[dev] starting judge-runtime on :9090 (dev token, no nsjail on this host) ..." -ForegroundColor Yellow
$env:ORANGEOJ_JUDGE_SHARED_TOKEN = "dev-token"
$env:ORANGEOJ_JUDGE_RUNTIME_PORT = "9090"
$judge = $null
$judgeBin = Join-Path $env:TEMP "orangeoj-judge-dev.exe"
Push-Location $root
go build -o $judgeBin ./cmd/judge-runtime
Pop-Location
if (Test-Path $judgeBin) {
  $judge = Start-Process -FilePath $judgeBin -WorkingDirectory $root -PassThru -NoNewWindow
} else {
  Write-Host "[dev] judge-runtime build failed; continuing without judge (run/test/submit disabled)." -ForegroundColor Yellow
}

Write-Host "[dev] starting OrangeOJ backend on :8080 (single process, data dir: $root\data) ..." -ForegroundColor Yellow
$goArgs = @("run", ".", "-data", (Join-Path $root "data"))
if ($judge) { $goArgs += @("-judge-endpoint", "http://127.0.0.1:9090", "-judge-token", "dev-token") }
$go = Start-Process -FilePath "go" -ArgumentList $goArgs -WorkingDirectory $root -PassThru -NoNewWindow

Write-Host "[dev] starting single frontend (app) Vite on :5175 (/api proxied to 8080) ..." -ForegroundColor Yellow
$appDir = Join-Path $root "app"
if (-not (Test-Path (Join-Path $appDir "node_modules"))) {
  Push-Location $appDir
  npm install
  Pop-Location
}
$env:PORT = "5175"
$fe = Start-Process -FilePath "npm.cmd" -ArgumentList "run", "dev" -WorkingDirectory $appDir -PassThru -NoNewWindow
Remove-Item Env:PORT -ErrorAction SilentlyContinue

Write-Host ""
Write-Host "[dev] ready? http://localhost:5175 (portal / + management /admin in one app; default password: 123456)" -ForegroundColor Green
Write-Host "[dev] press Ctrl+C to stop all."

try {
  Wait-Process -Id $go.Id -ErrorAction SilentlyContinue
} finally {
  foreach ($p in @($fe, $go, $judge)) {
    if ($p -and -not $p.HasExited) {
      taskkill /PID $p.Id /T /F 2>&1 | Out-Null
    }
  }
}
