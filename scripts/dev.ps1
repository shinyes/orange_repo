# OrangeRepo 开发脚本：并发启动 Go 后端与 Vite 前端。
# 用法：.\scripts\dev.ps1
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

Write-Host "[dev] 启动 Go 后端 :8080（数据目录 $root\data）..." -ForegroundColor Yellow
$go = Start-Process -FilePath "go" -ArgumentList "run", "." -WorkingDirectory $root -PassThru `
  -NoNewWindow

Write-Host "[dev] 启动 Vite 前端 :5173（/api 代理到 8080）..." -ForegroundColor Yellow
$web = Start-Process -FilePath "npm" -ArgumentList "run", "dev" -WorkingDirectory (Join-Path $root "web") `
  -PassThru -NoNewWindow

Write-Host ""
Write-Host "[dev] 就绪后访问 http://localhost:5173 （默认密码 123456）" -ForegroundColor Green
Write-Host "[dev] 按 Ctrl+C 退出（或关闭本窗口）。"

try {
  Wait-Process -Id $go.Id -ErrorAction SilentlyContinue
} finally {
  foreach ($p in @($web, $go)) {
    if ($p -and -not $p.HasExited) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue }
  }
}
