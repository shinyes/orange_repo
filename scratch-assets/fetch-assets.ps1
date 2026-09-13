# 批量下载 Scratch 素材库（Windows PowerShell）
#
# 用途：某些网络下 CI 抓不到 assets.scratch.mit.edu，可在能联网的机器上先抓一次，
#       再把结果交给 OrangeOJ（放进 scratch-assets/ 目录，或打成素材包挂到 Release）。
#
# 用法（在本目录下）：
#   pwsh -File fetch-assets.ps1                 # 默认下载到当前目录，8 并发
#   pwsh -File fetch-assets.ps1 -Out D:\assets -Jobs 16
#   pwsh -File fetch-assets.ps1 -Retry          # 只补下缺失/失败的文件
#
# 清单来源：仓库根目录执行
#   go run ./cmd/scratchassets -list scratch-assets/manifest.tsv
# （清单里每行是 URL<TAB>文件名）
param(
  [string]$Manifest = (Join-Path $PSScriptRoot 'manifest.tsv'),
  [string]$Out = $PSScriptRoot,
  [int]$Jobs = 8,
  [switch]$Retry
)

$ErrorActionPreference = 'Stop'
if (-not (Test-Path $Manifest)) { Write-Host "找不到清单：$Manifest" -ForegroundColor Red; exit 1 }
New-Item -ItemType Directory -Path $Out -Force | Out-Null

$items = Get-Content $Manifest | Where-Object { $_ -match "`t" } | ForEach-Object {
  $parts = $_ -split "`t", 2
  [pscustomobject]@{ Url = $parts[0].Trim(); Name = $parts[1].Trim() }
}
Write-Host ("清单条目：" + $items.Count + "；输出目录：" + $Out)

$todo = if ($Retry) { $items | Where-Object { -not (Test-Path (Join-Path $Out $_.Name)) } } else { $items }
Write-Host ("待下载：" + @($todo).Count)

# 进度与计数（PowerShell 5.1/7 均可用）
$script:ok = 0; $script:skip = 0; $script:fail = 0
$failed = [System.Collections.Concurrent.ConcurrentBag[string]]::new()

$todo | ForEach-Object -Parallel {
  $dst = Join-Path $using:Out $_.Name
  if (Test-Path $dst) { $script:skip++; return }
  try {
    Invoke-WebRequest -Uri $_.Url -OutFile $dst -TimeoutSec 60
    $script:ok++
  } catch {
    $script:fail++
    $failed.Add($_.Name + ' <- ' + $_.Url + ' : ' + $_.Exception.Message)
  }
} -ThrottleLimit $Jobs

Write-Host ("完成：新下载 " + $script:ok + "，已存在 " + $script:skip + "，失败 " + $script:fail)
if ($failed.Count -gt 0) {
  Write-Host "失败明细（前 20 条）：" -ForegroundColor Yellow
  $failed | Select-Object -First 20 | ForEach-Object { Write-Host "  $_" }
  Write-Host "可用 -Retry 只补下缺失项" -ForegroundColor Yellow
}

$total = (Get-ChildItem $Out -File | Measure-Object -Property Length -Sum)
Write-Host ("目录文件数：" + $total.Count + "，合计 " + [math]::Round($total.Sum / 1MB, 1) + " MB")
Write-Host ""
Write-Host "接下来（二选一）：" -ForegroundColor Cyan
Write-Host "  A) 把本目录（除 README.md/manifest.tsv/fetch-assets.* 外的素材文件）拷到仓库 scratch-assets/ 后构建："
Write-Host "     docker build --build-arg ASSET_MIRROR_REQUIRED=1 -f scratch/Dockerfile -t orangeoj-scratch ."
Write-Host "  B) 打成素材包上传到 GitHub Release，再用 URL 构建（CI 也能用）："
Write-Host "     tar -czf scratch-assets.tar.gz -C $Out ."
Write-Host "     docker build --build-arg ASSET_BUNDLE_URL=<素材包URL> -f scratch/Dockerfile -t orangeoj-scratch ."
