# OrangeRepo dev script (quiz edition): start main stack (:8080/:5173) + quiz service (:8081/:5174) + judge-runtime (:9090) together.
# Usage: .\scripts\dev-quiz.ps1
# NOTE: kept ASCII-only on purpose - PowerShell 5.1 misparses BOM-less UTF-8 scripts.
# npm.cmd is used explicitly because Start-Process "npm" resolves to the
# extension-less bash shim, which is not a Win32 executable.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

Write-Host "[dev] starting OrangeRepo backend on :8080 (data dir: $root\data) ..." -ForegroundColor Yellow
$main = Start-Process -FilePath "go" -ArgumentList "run", "." -WorkingDirectory $root -PassThru -NoNewWindow

# Judge runtime: sandbox executor (Linux containers use nsjail; local runs are process-limited, dev only).
Write-Host "[dev] starting OrangeOJ judge-runtime on :9090 (dev token, no nsjail on this host) ..." -ForegroundColor Yellow
$env:ORANGEOJ_JUDGE_SHARED_TOKEN = "dev-token"
$judge = Start-Process -FilePath "go" -ArgumentList "run", "./cmd/judge-runtime" -WorkingDirectory $root -PassThru -NoNewWindow

Write-Host "[dev] starting Orange quiz backend on :8081 (same data dir, judge connected) ..." -ForegroundColor Yellow
$quiz = Start-Process -FilePath "go" -ArgumentList "run", "./cmd/quiz", "-judge-endpoint", "http://127.0.0.1:9090", "-judge-token", "dev-token", "-judge-workers", "2" -WorkingDirectory $root -PassThru -NoNewWindow

Write-Host "[dev] starting OrangeRepo frontend on :5173 (/api proxied to 8080) ..." -ForegroundColor Yellow
$webDir = Join-Path $root "web"
if (-not (Test-Path (Join-Path $webDir "node_modules"))) {
  Write-Host "[dev] web/node_modules missing, running npm install first ..." -ForegroundColor Yellow
  Push-Location $webDir
  npm install
  Pop-Location
}
$web = Start-Process -FilePath "npm.cmd" -ArgumentList "run", "dev" -WorkingDirectory $webDir -PassThru -NoNewWindow

Write-Host "[dev] starting Orange quiz frontend on :5174 (/api proxied to 8081) ..." -ForegroundColor Yellow
$quizDir = Join-Path $root "web-quiz"
if (-not (Test-Path (Join-Path $quizDir "node_modules"))) {
  Write-Host "[dev] web-quiz/node_modules missing, running npm install first ..." -ForegroundColor Yellow
  Push-Location $quizDir
  npm install
  Pop-Location
}
$quizWeb = Start-Process -FilePath "npm.cmd" -ArgumentList "run", "dev" -WorkingDirectory $quizDir -PassThru -NoNewWindow

# Wait-Health polls a backend /api/health until ready (go run needs to compile first).
function Wait-Health($url, $name, $n = 60) {
  for ($i = 0; $i -lt $n; $i++) {
    try {
      $r = Invoke-RestMethod $url -TimeoutSec 1
      if ($r.ok -or $r.status) { Write-Host "[dev] $name ready" -ForegroundColor Green; return }
    } catch { Start-Sleep -Milliseconds 500 }
  }
  Write-Host "[dev] WARN: $name not responding at $url - see error output above" -ForegroundColor Red
}

# Wait for both Go backends (compilation takes time; avoids ECONNREFUSED noise in Vite).
Wait-Health 'http://127.0.0.1:8080/api/health' 'OrangeRepo backend'
Wait-Health 'http://127.0.0.1:9090/healthz' 'judge runtime'
Wait-Health 'http://127.0.0.1:8081/api/health' 'Orange quiz backend'

Write-Host ""
Write-Host "[dev] main app:  http://localhost:5173  (default password: 123456)" -ForegroundColor Green
Write-Host "[dev] quiz app:  http://localhost:5174  (default admin: admin/123456)" -ForegroundColor Green
Write-Host "[dev] judge runtime: http://localhost:9090 (internal; dev token 'dev-token')" -ForegroundColor Green
Write-Host "[dev] press Ctrl+C to stop all servers." -ForegroundColor Green

try {
  Wait-Process -Id $main.Id -ErrorAction SilentlyContinue
} finally {
  # Sub-processes may exit on their own (e.g. port already in use);
  # taskkill errors on already-dead PIDs are expected and silenced.
  $ErrorActionPreference = 'SilentlyContinue'
  # /T kills the whole process tree (go run spawns a child exe; npm spawns node)
  foreach ($p in @($quizWeb, $web, $quiz, $judge, $main)) {
    if ($p -and -not $p.HasExited) {
      taskkill /PID $p.Id /T /F 2>&1 | Out-Null
    }
  }
}
