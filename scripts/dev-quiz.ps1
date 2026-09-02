# OrangeRepo dev script (quiz edition): start main stack (:8080/:5173) + quiz service (:8081/:5174) together.
# Usage: .\scripts\dev-quiz.ps1
# NOTE: kept ASCII-only on purpose - PowerShell 5.1 misparses BOM-less UTF-8 scripts.
# npm.cmd is used explicitly because Start-Process "npm" resolves to the
# extension-less bash shim, which is not a Win32 executable.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

Write-Host "[dev] starting OrangeRepo backend on :8080 (data dir: $root\data) ..." -ForegroundColor Yellow
$main = Start-Process -FilePath "go" -ArgumentList "run", "." -WorkingDirectory $root -PassThru -NoNewWindow

Write-Host "[dev] starting OrangeQuiz backend on :8081 (same data dir) ..." -ForegroundColor Yellow
$quiz = Start-Process -FilePath "go" -ArgumentList "run", "./cmd/quiz" -WorkingDirectory $root -PassThru -NoNewWindow

Write-Host "[dev] starting OrangeRepo frontend on :5173 (/api proxied to 8080) ..." -ForegroundColor Yellow
$webDir = Join-Path $root "web"
if (-not (Test-Path (Join-Path $webDir "node_modules"))) {
  Write-Host "[dev] web/node_modules missing, running npm install first ..." -ForegroundColor Yellow
  Push-Location $webDir
  npm install
  Pop-Location
}
$web = Start-Process -FilePath "npm.cmd" -ArgumentList "run", "dev" -WorkingDirectory $webDir -PassThru -NoNewWindow

Write-Host "[dev] starting OrangeQuiz frontend on :5174 (/api proxied to 8081) ..." -ForegroundColor Yellow
$quizDir = Join-Path $root "web-quiz"
if (-not (Test-Path (Join-Path $quizDir "node_modules"))) {
  Write-Host "[dev] web-quiz/node_modules missing, running npm install first ..." -ForegroundColor Yellow
  Push-Location $quizDir
  npm install
  Pop-Location
}
$quizWeb = Start-Process -FilePath "npm.cmd" -ArgumentList "run", "dev" -WorkingDirectory $quizDir -PassThru -NoNewWindow

Write-Host ""
Write-Host "[dev] main app:  http://localhost:5173  (default password: 123456)" -ForegroundColor Green
Write-Host "[dev] quiz app:  http://localhost:5174  (default admin: admin/123456)" -ForegroundColor Green
Write-Host "[dev] press Ctrl+C to stop both servers." -ForegroundColor Green

try {
  Wait-Process -Id $main.Id -ErrorAction SilentlyContinue
} finally {
  # 子进程可能已自行退出（如端口被占用时），taskkill 报错属正常，静默处理
  $ErrorActionPreference = 'SilentlyContinue'
  # /T kills the whole process tree (go run spawns a child exe; npm spawns node)
  foreach ($p in @($quizWeb, $web, $quiz, $main)) {
    if ($p -and -not $p.HasExited) {
      taskkill /PID $p.Id /T /F 2>&1 | Out-Null
    }
  }
}