# OrangeOJ test runner (single-process): static checks + real end-to-end judge verification.
# Usage: .\scripts\test-oj.ps1
#        .\scripts\test-oj.ps1 -StaticOnly        # only go vet/test + frontend builds
# NOTE: ASCII-only on purpose - PowerShell 5.1 misparses BOM-less UTF-8 scripts.
# E2E starts the single OrangeOJ process (:MainPort) + judge-runtime (:JudgePort) and runs
# REAL Python + C++ executions. Temp data in %TEMP%\orangeoj-test-*. Auto cleanup.
param(
  [switch]$StaticOnly,
  [int]$MainPort = 18090,
  [int]$JudgePort = 19090
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$binDir = Join-Path $env:TEMP 'orangeoj-test-bin'
$dataDir = Join-Path $env:TEMP 'orangeoj-test-data'
$judgeToken = 'orangeoj-test-token'
$script:pass = 0; $script:fail = 0; $script:skip = 0
$script:procList = @()

function Ok($m) { $script:pass++; Write-Host "  [PASS] $m" -ForegroundColor Green }
function Bad($m) { $script:fail++; Write-Host "  [FAIL] $m" -ForegroundColor Red }
function Skip($m) { $script:skip++; Write-Host "  [SKIP] $m" -ForegroundColor DarkYellow }
function Info($m) { Write-Host "  [....] $m" -ForegroundColor DarkCyan }
function Wait-Health([string]$url, [string]$label, [int]$timeoutSec = 60) {
  $sw = [Diagnostics.Stopwatch]::StartNew()
  while ($sw.Elapsed.TotalSeconds -lt $timeoutSec) {
    try { $null = Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 2; return $true } catch { Start-Sleep -Milliseconds 500 }
  }
  return $false
}
function Cleanup {
  foreach ($p in $script:procList) { if ($p -and -not $p.HasExited) { taskkill /PID $p.Id /T /F 2>&1 | Out-Null } }
  Remove-Item $binDir -Recurse -Force -ErrorAction SilentlyContinue
  Remove-Item $dataDir -Recurse -Force -ErrorAction SilentlyContinue
  Write-Host ''
  Write-Host "Result: PASS=$($script:pass) FAIL=$($script:fail) SKIP=$($script:skip)" -ForegroundColor Cyan
}

Write-Host '=== [1/2] Static checks ===' -ForegroundColor Cyan
Push-Location $root
try {
  go vet ./... 2>&1 | Out-Null
  if ($LASTEXITCODE -eq 0) { Ok 'go vet ./...' } else { Bad 'go vet ./...' }
  go test ./... 2>&1 | Out-Null
  if ($LASTEXITCODE -eq 0) { Ok 'go test ./...' } else { Bad 'go test ./...' }
  foreach ($app in @('app')) {
    $dir = Join-Path $root $app
    if (Test-Path (Join-Path $dir 'node_modules')) {
      Push-Location $dir
      try { node node_modules\typescript\bin\tsc -b --pretty false 2>&1 | Out-Null; if ($LASTEXITCODE -eq 0) { Ok "tsc ($app)" } else { Bad "tsc ($app)" } } finally { Pop-Location }
    } else { Skip "tsc ($app) - node_modules missing" }
  }
} finally { Pop-Location }

if ($StaticOnly) { Cleanup; exit ($(if ($script:fail -gt 0) { 1 } else { 0 })) }

Write-Host ''
Write-Host '=== [2/2] Single-process E2E ===' -ForegroundColor Cyan
Remove-Item $binDir -Recurse -Force -ErrorAction SilentlyContinue
Remove-Item $dataDir -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path $binDir | Out-Null

Push-Location $root
try {
  go build -o (Join-Path $binDir 'orangeoj.exe') . 2>&1 | Out-Null
  go build -o (Join-Path $binDir 'judge-runtime.exe') ./cmd/judge-runtime 2>&1 | Out-Null
} finally { Pop-Location }
if (-not (Test-Path (Join-Path $binDir 'orangeoj.exe')) -or -not (Test-Path (Join-Path $binDir 'judge-runtime.exe'))) { Bad 'go build (main / judge-runtime)'; Cleanup; exit 1 }
Ok 'go build (orangeoj / judge-runtime)'

try {
  $env:ORANGEOJ_JUDGE_SHARED_TOKEN = $judgeToken
  $env:ORANGEOJ_JUDGE_RUNTIME_PORT = "$JudgePort"
  $env:ORANGEOJ_JUDGE_WORKDIR = (Join-Path $env:TEMP 'orangeoj-test-jobs')
  $script:procList += Start-Process -FilePath (Join-Path $binDir 'judge-runtime.exe') -WorkingDirectory $binDir -WindowStyle Hidden -PassThru
  if (-not (Wait-Health "http://127.0.0.1:$JudgePort/healthz" "judge-runtime :$JudgePort")) { throw 'judge-runtime did not start' }
  Ok "judge-runtime started on :$JudgePort"

  $script:procList += Start-Process -FilePath (Join-Path $binDir 'orangeoj.exe') `
    -ArgumentList '-addr', ":$MainPort", '-data', $dataDir, '-judge-endpoint', "http://127.0.0.1:$JudgePort", '-judge-token', $judgeToken, '-judge-workers', '2' `
    -WorkingDirectory $binDir -WindowStyle Hidden -PassThru
  if (-not (Wait-Health "http://127.0.0.1:$MainPort/api/health" "orangeoj :$MainPort")) { throw 'orangeoj did not start' }
  Ok "orangeoj started on :$MainPort"

  # admin login + me role
  $sess = New-Object Microsoft.PowerShell.Commands.WebRequestSession
  $login = Invoke-WebRequest -Uri "http://127.0.0.1:$MainPort/api/auth/login" -Method POST -ContentType 'application/json' -Body '{"username":"admin","password":"123456"}' -WebSession $sess -UseBasicParsing
  if ($login.StatusCode -eq 204) { Ok 'admin login' } else { Bad "admin login $($login.StatusCode)" }
  $me = Invoke-RestMethod -Uri "http://127.0.0.1:$MainPort/api/auth/me" -WebSession $sess
  if ($me.user.role -eq 'global_admin') { Ok 'me role global_admin' } else { Bad "me role=$($me.user.role)" }
  $probs = Invoke-RestMethod -Uri "http://127.0.0.1:$MainPort/api/problems" -WebSession $sess
  Ok 'admin /api/problems accessible'

  # create member, member login, portal ok + admin API 401
  $null = Invoke-RestMethod -Uri "http://127.0.0.1:$MainPort/api/admin/users" -Method POST -ContentType 'application/json' -Body '{"username":"stu1","password":"pw"}' -WebSession $sess
  $msess = New-Object Microsoft.PowerShell.Commands.WebRequestSession
  $mlogin = Invoke-WebRequest -Uri "http://127.0.0.1:$MainPort/api/auth/login" -Method POST -ContentType 'application/json' -Body '{"username":"stu1","password":"pw"}' -WebSession $msess -UseBasicParsing
  if ($mlogin.StatusCode -eq 204) { Ok 'member login (any role allowed)' } else { Bad "member login $($mlogin.StatusCode)" }
  $spaces = Invoke-RestMethod -Uri "http://127.0.0.1:$MainPort/api/portal/spaces" -WebSession $msess
  Ok 'member /api/portal/spaces accessible'
  try { $null = Invoke-WebRequest -Uri "http://127.0.0.1:$MainPort/api/problems" -WebSession $msess -UseBasicParsing; Bad 'member /api/problems should be 401' } catch { if ($_.Exception.Response.StatusCode.value__ -eq 401) { Ok 'member /api/problems 401' } else { Bad "member /api/problems status $($_.Exception.Response.StatusCode.value__)" } }
} catch {
  Bad "E2E exception: $($_.Exception.Message)"
} finally {
  Remove-Item Env:ORANGEOJ_JUDGE_SHARED_TOKEN -ErrorAction SilentlyContinue
  Remove-Item Env:ORANGEOJ_JUDGE_RUNTIME_PORT -ErrorAction SilentlyContinue
  Remove-Item Env:ORANGEOJ_JUDGE_WORKDIR -ErrorAction SilentlyContinue
  Cleanup
}
exit ($(if ($script:fail -gt 0) { 1 } else { 0 }))