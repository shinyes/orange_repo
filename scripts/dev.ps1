# OrangeRepo dev script: start Go backend (:8080) + Vite frontend (:5173) together.
# Usage: .\scripts\dev.ps1
# NOTE: kept ASCII-only on purpose - PowerShell 5.1 misparses BOM-less UTF-8 scripts.
# npm.cmd is used explicitly because Start-Process "npm" resolves to the
# extension-less bash shim, which is not a Win32 executable.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

Write-Host "[dev] starting Go backend on :8080 (data dir: $root\data) ..." -ForegroundColor Yellow
$go = Start-Process -FilePath "go" -ArgumentList "run", "." -WorkingDirectory $root -PassThru -NoNewWindow

Write-Host "[dev] starting Vite frontend on :5173 (/api proxied to 8080) ..." -ForegroundColor Yellow
$webDir = Join-Path $root "web"
if (-not (Test-Path (Join-Path $webDir "node_modules"))) {
  Write-Host "[dev] web/node_modules missing, running npm install first ..." -ForegroundColor Yellow
  Push-Location $webDir
  npm install
  Pop-Location
}
$web = Start-Process -FilePath "npm.cmd" -ArgumentList "run", "dev" -WorkingDirectory $webDir -PassThru -NoNewWindow

Write-Host ""
Write-Host "[dev] ready? open http://localhost:5173  (default password: 123456)" -ForegroundColor Green
Write-Host "[dev] press Ctrl+C to stop both servers."

try {
  Wait-Process -Id $go.Id -ErrorAction SilentlyContinue
} finally {
  # /T kills the whole process tree (go run spawns a child exe; npm spawns node)
  foreach ($p in @($web, $go)) {
    if ($p -and -not $p.HasExited) {
      taskkill /PID $p.Id /T /F 2>&1 | Out-Null
    }
  }
}
