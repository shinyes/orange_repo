# 批量下载 Scratch 素材库（Windows；兼容 Windows PowerShell 5.1 与 PowerShell 7）
#
# 用途：某些网络/CI 环境抓不到 assets.scratch.mit.edu，可在能联网的机器上先抓一次，
#       再把结果交给 OrangeOJ（放进 scratch-assets/，或打成素材包挂到 Release）。
#
# 用法（在仓库根目录）：
#   powershell -ExecutionPolicy Bypass -File scratch-assets\fetch-assets.ps1 -Out D:\scratch-assets
#   powershell -ExecutionPolicy Bypass -File scratch-assets\fetch-assets.ps1 -Out D:\scratch-assets -Jobs 8
#   powershell -ExecutionPolicy Bypass -File scratch-assets\fetch-assets.ps1 -Out D:\scratch-assets -Retry
#   （装了 PowerShell 7 的话，把 powershell 换成 pwsh 亦可）
#
# 说明：
#   · 默认顺序下载（最稳）；-Jobs 8 用后台作业并发（分片经 CSV 文件传递，避开 5.1 的数组序列化坑）
#   · 已存在且非空的文件自动跳过；-Retry 只补缺失项，可反复执行
#   · 本文件必须保存为「UTF-8 带 BOM」，否则 Windows PowerShell 5.1 会把中文按 ANSI 解析而报语法错误
param(
  [string]$Manifest = (Join-Path $PSScriptRoot 'manifest.tsv'),
  [string]$Out = $PSScriptRoot,
  [int]$Jobs = 1,
  [switch]$Retry
)

$ErrorActionPreference = 'Stop'
if (-not (Test-Path -LiteralPath $Manifest)) { Write-Host "找不到清单：$Manifest" -ForegroundColor Red; exit 1 }
New-Item -ItemType Directory -Path $Out -Force | Out-Null

$items = New-Object System.Collections.ArrayList
foreach ($line in (Get-Content -LiteralPath $Manifest)) {
  $parts = $line -split "`t", 2
  if ($parts.Count -lt 2) { continue }
  $null = $items.Add([pscustomobject]@{ Url = $parts[0].Trim(); Name = $parts[1].Trim() })
}
Write-Host ("清单条目：{0}；输出目录：{1}" -f $items.Count, $Out)

$todo = @($items)
if ($Retry) {
  $todo = @($items | Where-Object {
    $p = Join-Path $Out $_.Name
    -not (Test-Path -LiteralPath $p) -or ((Get-Item -LiteralPath $p).Length -eq 0)
  })
}
Write-Host ("待下载：{0}" -f $todo.Count)
if ($todo.Count -eq 0) { Write-Host "没有待下载文件（全部已就绪）" -ForegroundColor Green; exit 0 }

# 下载逻辑（顺序与并行共用）：入参 items 为含 Url/Name 的对象数组
$downloadBody = {
  param($items, $out)
  $ok = 0; $skip = 0; $fail = 0
  $fails = New-Object System.Collections.ArrayList
  $wc = New-Object System.Net.WebClient
  try { $wc.Proxy = [System.Net.WebRequest]::DefaultWebProxy } catch { }
  foreach ($it in $items) {
    $name = [string]$it.Name
    $url = [string]$it.Url
    $dst = Join-Path $out $name
    if ((Test-Path -LiteralPath $dst) -and ((Get-Item -LiteralPath $dst).Length -gt 0)) { $skip++; continue }
    $tmp = "$dst.part"
    try {
      $wc.DownloadFile($url, $tmp)
      if ((Get-Item -LiteralPath $tmp).Length -eq 0) { throw "空文件" }
      Move-Item -LiteralPath $tmp -Destination $dst -Force
      $ok++
    } catch {
      $fail++
      if ($fails.Count -lt 20) { $null = $fails.Add($name + ' <- ' + $_.Exception.Message) }
      if (Test-Path -LiteralPath $tmp) { Remove-Item -LiteralPath $tmp -Force -ErrorAction SilentlyContinue }
    }
  }
  [pscustomobject]@{ Ok = $ok; Skip = $skip; Fail = $fail; Fails = $fails }
}

$results = @()
if ($Jobs -le 1) {
  Write-Host "顺序下载中（-Jobs N 可并发）…"
  $results = @(& $downloadBody -items $todo -out $Out)
} else {
  $jobCount = [Math]::Max(1, [Math]::Min($Jobs, 32))
  $chunks = @()
  for ($i = 0; $i -lt $jobCount; $i++) {
    $c = @($todo | Where-Object { $todo.IndexOf($_) % $jobCount -eq $i })
    if ($c.Count -gt 0) { $chunks += , $c }
  }
  Write-Host ("并发作业数：{0}（分片经 CSV 文件传递）" -f $chunks.Count)
  $slices = @()
  $chunkFiles = @()
  $idx = 0
  foreach ($chunk in $chunks) {
    $idx++
    $chunkFile = Join-Path $env:TEMP ("orangeoj-assets-chunk-{0}-{1}.tsv" -f $PID, $idx)
    $chunk | Export-Csv -LiteralPath $chunkFile -Delimiter "`t" -NoTypeInformation -Encoding UTF8
    $chunkFiles += $chunkFile
    $slices += , @{ File = $chunkFile; Out = $Out }
  }
  $running = @()
  foreach ($s in $slices) {
    # 作业内从 CSV 读分片，并把下载逻辑**内联**（不传脚本块：Windows PowerShell 5.1
    # 无法把 ScriptBlock 跨进程序列化进作业，传了会静默失败——本脚本踩过这个坑）
    $running += Start-Job -ScriptBlock {
      param($chunkFile, $outDir)
      $items = @(Import-Csv -LiteralPath $chunkFile -Delimiter "`t")
      $ok = 0; $skip = 0; $fail = 0
      $fails = New-Object System.Collections.ArrayList
      $wc = New-Object System.Net.WebClient
      try { $wc.Proxy = [System.Net.WebRequest]::DefaultWebProxy } catch { }
      foreach ($it in $items) {
        $name = [string]$it.Name
        $url = [string]$it.Url
        $dst = Join-Path $outDir $name
        if ((Test-Path -LiteralPath $dst) -and ((Get-Item -LiteralPath $dst).Length -gt 0)) { $skip++; continue }
        $tmp = "$dst.part"
        try {
          $wc.DownloadFile($url, $tmp)
          if ((Get-Item -LiteralPath $tmp).Length -eq 0) { throw "空文件" }
          Move-Item -LiteralPath $tmp -Destination $dst -Force
          $ok++
        } catch {
          $fail++
          if ($fails.Count -lt 20) { $null = $fails.Add($name + ' <- ' + $_.Exception.Message) }
          if (Test-Path -LiteralPath $tmp) { Remove-Item -LiteralPath $tmp -Force -ErrorAction SilentlyContinue }
        }
      }
      [pscustomobject]@{ Ok = $ok; Skip = $skip; Fail = $fail; Fails = $fails }
    } -ArgumentList $s.File, $s.Out
  }
  Write-Host "下载中…（完成前请不要关闭窗口）"
  $null = Wait-Job -Job $running
  $results = @(Receive-Job -Job $running)
  Remove-Job -Job $running -Force
  foreach ($f in $chunkFiles) { Remove-Item -LiteralPath $f -Force -ErrorAction SilentlyContinue }
}

$ok = ($results | Measure-Object -Property Ok -Sum).Sum
$skip = ($results | Measure-Object -Property Skip -Sum).Sum
$fail = ($results | Measure-Object -Property Fail -Sum).Sum
Write-Host ("完成：新下载 {0}，已存在 {1}，失败 {2}" -f $ok, $skip, $fail)
$allFails = @($results | ForEach-Object { $_.Fails } | Select-Object -First 20)
if ($allFails.Count -gt 0) {
  Write-Host "失败明细（前 20 条）：" -ForegroundColor Yellow
  $allFails | ForEach-Object { Write-Host "  $_" }
  Write-Host "可用 -Retry 只补下缺失项" -ForegroundColor Yellow
}

$files = @(Get-ChildItem -Path $Out -File | Where-Object { $_.Extension -ne '.tsv' -and $_.Name -notlike 'fetch-assets*' -and $_.Name -ne 'README.md' })
$sum = 0
if ($files.Count -gt 0) { $sum = ($files | Measure-Object -Property Length -Sum).Sum }
Write-Host ("目录素材文件数：{0}，合计 {1} MB" -f $files.Count, [math]::Round($sum / 1MB, 1))
Write-Host ""
Write-Host "接下来（二选一）：" -ForegroundColor Cyan
Write-Host "  A) 把素材文件拷到仓库 scratch-assets\ 后构建："
Write-Host "     docker build --build-arg ASSET_MIRROR_REQUIRED=1 -f scratch/Dockerfile -t orangeoj-scratch ."
Write-Host "  B) 打成素材包上传到 GitHub Release，再用 URL 构建（CI 也能用）："
Write-Host "     tar -czf scratch-assets.tar.gz -C $Out ."
Write-Host "     docker build --build-arg ASSET_BUNDLE_URL=<素材包URL> -f scratch/Dockerfile -t orangeoj-scratch ."
