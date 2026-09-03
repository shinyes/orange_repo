# OrangeOJ test runner: static checks + real end-to-end judge verification.
# Usage:  .\scripts\test-oj.ps1
#         .\scripts\test-oj.ps1 -StaticOnly          # only go vet/test + frontend builds
#         .\scripts\test-oj.ps1 -SkipFrontends       # skip npm builds
#         .\scripts\test-oj.ps1 -MainPort 18090 -QuizPort 18091 -JudgePort 19090
# NOTE: kept ASCII-only on purpose - PowerShell 5.1 misparses BOM-less UTF-8 scripts.
# E2E uses three real processes (main repo :MainPort / quiz+OJ :QuizPort / judge :JudgePort)
# and REAL Python + C++ executions through judge-runtime. All temp data goes to %TEMP%\orangeoj-test-*.
# C++ assertions are skipped (with a warning) when no local g++ toolchain is found.
param(
  [switch]$StaticOnly,
  [switch]$SkipFrontends,
  [int]$MainPort = 18090,
  [int]$QuizPort = 18091,
  [int]$JudgePort = 19090
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$binDir = Join-Path $env:TEMP 'orangeoj-test-bin'
$dataDir = Join-Path $env:TEMP 'orangeoj-test-data'
$judgeToken = 'orangeoj-test-token'

$script:pass = 0
$script:fail = 0
$script:skip = 0
$script:procList = @()
$script:lastErr = ''
$script:lastStatus = 0

function Note($m)   { Write-Host "[NOTE] $m" -ForegroundColor DarkYellow }
function Info($m)   { Write-Host "[INFO] $m" -ForegroundColor Gray }
function Ok($m)     { Write-Host "[PASS] $m" -ForegroundColor Green;  $script:pass++ }
function Bad($m)    { Write-Host "[FAIL] $m" -ForegroundColor Red;    $script:fail++
                      if ($script:lastErr) { Write-Host "       $($script:lastErr)" -ForegroundColor DarkRed } }
function Skip($m)   { Write-Host "[SKIP] $m" -ForegroundColor DarkYellow; $script:skip++ }

function Wait-Health($url, $name, $n = 90) {
  for ($i = 0; $i -lt $n; $i++) {
    try {
      $r = Invoke-RestMethod $url -TimeoutSec 1
      if ($r.ok -or $r.status) { Info "$name ready"; return $true }
    } catch { }
    Start-Sleep -Milliseconds 500
  }
  Note "$name NOT responding at $url"
  return $false
}

# HTTP helper: returns Invoke-WebRequest response or $null; sets $script:lastStatus / $script:lastErr.
function Api($method, $url, $body, $sess) {
  $script:lastStatus = 0
  $script:lastErr = ''
  try {
    if ($null -ne $body) {
      $resp = Invoke-WebRequest -Uri $url -Method $method -Body ($body | ConvertTo-Json -Depth 8) `
        -ContentType 'application/json; charset=utf-8' -WebSession $sess
    } else {
      $resp = Invoke-WebRequest -Uri $url -Method $method -WebSession $sess
    }
    $script:lastStatus = [int]$resp.StatusCode
    return $resp
  } catch {
    if ($_.Exception.Response) { $script:lastStatus = [int]$_.Exception.Response.StatusCode }
    $script:lastErr = $_.ErrorDetails.Message
    if (-not $script:lastErr) { $script:lastErr = $_.Exception.Message }
    return $null
  }
}

function JsonOf($resp) {
  if ($null -eq $resp) { return $null }
  try { return $resp.Content | ConvertFrom-Json } catch { return $null }
}

function Login($base, $user, $pass) {
  $s = New-Object Microsoft.PowerShell.Commands.WebRequestSession
  $resp = Api 'POST' "$base/api/auth/login" @{ username = $user; password = $pass } $s
  if ($script:lastStatus -eq 204) { return $s }
  return $null
}

function Poll-Judge($base, $sess, $subId) {
  for ($i = 0; $i -lt 120; $i++) {
    $r = JsonOf (Api 'GET' "$base/api/oj/submission/$subId/poll" $null $sess)
    if ($r -and $r.isFinal) { return $r }
    Start-Sleep -Milliseconds 500
  }
  return $null
}

# Stop any process we started (best effort) and drop temp dirs.
function Cleanup {
  $ErrorActionPreference = 'SilentlyContinue'
  foreach ($p in $script:procList) {
    if ($p -and -not $p.HasExited) { taskkill /PID $p.Id /T /F 2>&1 | Out-Null }
  }
  Start-Sleep -Milliseconds 300
  Remove-Item $binDir -Recurse -Force -ErrorAction SilentlyContinue
  Remove-Item $dataDir -Recurse -Force -ErrorAction SilentlyContinue
  $ErrorActionPreference = 'Stop'
}

function Find-Gpp {
  $cmd = Get-Command g++ -ErrorAction SilentlyContinue
  if ($cmd -and $cmd.Source -notmatch 'WindowsApps') { return $cmd.Source }
  foreach ($p in @(
    'D:\tools\mingw\mingw64\bin\g++.exe',
    'C:\msys64\mingw64\bin\g++.exe',
    'C:\mingw64\bin\g++.exe',
    'C:\Program Files\mingw-w64\mingw64\bin\g++.exe'
  )) { if (Test-Path $p) { return $p } }
  return ''
}

# Probe real python interpreters (skip WindowsApps store stubs).
function Find-Python {
  $cmd = Get-Command python -ErrorAction SilentlyContinue
  if ($cmd -and $cmd.Source -notmatch 'WindowsApps') { return $cmd.Source }
  $cmd3 = Get-Command python3 -ErrorAction SilentlyContinue
  if ($cmd3 -and $cmd3.Source -notmatch 'WindowsApps') { return $cmd3.Source }
  foreach ($p in @(
    'C:\Python314\python.exe', 'C:\Python313\python.exe', 'C:\Python312\python.exe',
    'C:\Python311\python.exe', 'C:\Python310\python.exe'
  )) { if (Test-Path $p) { return $p } }
  return ''
}

$gppPath = Find-Gpp
$pyPath = Find-Python

# ============================================================ static checks
Write-Host ''
Write-Host '=== [1/3] Static checks ===' -ForegroundColor Cyan

Push-Location $root
try {
  Info 'go vet ./...'
  go vet ./... 2>&1 | Out-Null
  if ($LASTEXITCODE -eq 0) { Ok 'go vet ./...' } else { Bad 'go vet ./...' }

  Info 'go test ./... (incl. real python/cpp judge smoke)'
  go test ./... 2>&1 | Out-Null
  if ($LASTEXITCODE -eq 0) { Ok 'go test ./...' } else { Bad 'go test ./...' }

  Info 'GOOS=linux CGO_ENABLED=0 go build ./... (cross-compile incl. nsjail backend)'
  $env:GOOS = 'linux'; $env:CGO_ENABLED = '0'
  go build ./... 2>&1 | Out-Null
  $crossOk = ($LASTEXITCODE -eq 0)
  Remove-Item Env:GOOS -ErrorAction SilentlyContinue
  Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
  if ($crossOk) { Ok 'linux cross build ./...' } else { Bad 'linux cross build ./...' }
} finally {
  Pop-Location
}

if (-not $SkipFrontends) {
  foreach ($app in @('web', 'web-quiz')) {
    $dir = Join-Path $root $app
    if (Test-Path (Join-Path $dir 'node_modules')) {
      Info "npm run build ($app)"
      Push-Location $dir
      try { npm run build 2>&1 | Out-Null; if ($LASTEXITCODE -eq 0) { Ok "npm run build ($app)" } else { Bad "npm run build ($app)" } }
      finally { Pop-Location }
    } else {
      Skip "npm run build ($app) - node_modules missing, run 'cd $app; npm install' first"
    }
  }
} else {
  Skip 'frontend builds (-SkipFrontends)'
}

if ($StaticOnly) {
  Skip 'E2E (-StaticOnly)'
  Write-Host ''
  Write-Host "Result: PASS=$($script:pass) FAIL=$($script:fail) SKIP=$($script:skip)" -ForegroundColor Cyan
  exit ($(if ($script:fail -gt 0) { 1 } else { 0 }))
}

# ============================================================ build + start three processes
Write-Host ''
Write-Host '=== [2/3] Build binaries & start services ===' -ForegroundColor Cyan
Remove-Item $binDir -Recurse -Force -ErrorAction SilentlyContinue
Remove-Item $dataDir -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path $binDir | Out-Null

Push-Location $root
try {
  go build -o (Join-Path $binDir 'orangerepo.exe') . 2>&1 | Out-Null
  go build -o (Join-Path $binDir 'quiz.exe') ./cmd/quiz 2>&1 | Out-Null
  go build -o (Join-Path $binDir 'judge-runtime.exe') ./cmd/judge-runtime 2>&1 | Out-Null
} finally { Pop-Location }
if (-not (Test-Path (Join-Path $binDir 'judge-runtime.exe'))) { Bad 'go build (three binaries)'; Cleanup; exit 1 }
Ok 'go build (main / quiz / judge-runtime)'

try {
  $mainLog = Join-Path $env:TEMP 'orangeoj-test-main.log'
  $quizLog = Join-Path $env:TEMP 'orangeoj-test-quiz.log'
  $judgeLog = Join-Path $env:TEMP 'orangeoj-test-judge.log'

  # main repo server first (bootstraps shared admin into quiz.db)
  $script:procList += Start-Process -FilePath (Join-Path $binDir 'orangerepo.exe') `
    -ArgumentList '-addr', ":$MainPort", '-data', $dataDir, '-web', (Join-Path $root 'web\dist') `
    -WorkingDirectory $binDir -WindowStyle Hidden -RedirectStandardOutput $mainLog -RedirectStandardError "$mainLog.err" -PassThru
  if (-not (Wait-Health "http://127.0.0.1:$MainPort/api/health" "main repo :$MainPort")) { throw 'main repo did not start' }
  Ok "main repo started on :$MainPort"

  # judge runtime (dev backend when no nsjail)
  $env:ORANGEOJ_JUDGE_SHARED_TOKEN = $judgeToken
  $env:ORANGEOJ_JUDGE_RUNTIME_PORT = "$JudgePort"
  $script:procList += Start-Process -FilePath (Join-Path $binDir 'judge-runtime.exe') `
    -WorkingDirectory $binDir -WindowStyle Hidden -RedirectStandardOutput $judgeLog -RedirectStandardError "$judgeLog.err" -PassThru
  if (-not (Wait-Health "http://127.0.0.1:$JudgePort/healthz" "judge runtime :$JudgePort")) { throw 'judge runtime did not start' }
  Ok "judge-runtime started on :$JudgePort"

  # quiz / OJ service connected to judge
  $script:procList += Start-Process -FilePath (Join-Path $binDir 'quiz.exe') `
    -ArgumentList '-addr', ":$QuizPort", '-data', $dataDir, '-web', (Join-Path $root 'web-quiz\dist'),
    '-judge-endpoint', "http://127.0.0.1:$JudgePort", '-judge-token', $judgeToken, '-judge-workers', '2' `
    -WorkingDirectory $binDir -WindowStyle Hidden -RedirectStandardOutput $quizLog -RedirectStandardError "$quizLog.err" -PassThru
  if (-not (Wait-Health "http://127.0.0.1:$QuizPort/api/health" "quiz/OJ :$QuizPort")) { throw 'quiz service did not start' }
  Ok "quiz/OJ service started on :$QuizPort"

  $mainBase = "http://127.0.0.1:$MainPort"
  $quizBase = "http://127.0.0.1:$QuizPort"

  # ============================================================ E2E flow
  Write-Host ''
  Write-Host '=== [3/3] Real judge E2E ===' -ForegroundColor Cyan

  # --- seed main repo: A+B programming (3 testcases) + true/false, wrapped in one training ---
  $mAdmin = Login $mainBase 'admin' '123456'
  if (-not $mAdmin) { throw 'main admin login failed' }
  Ok 'main admin login'

  $p = @{
    type = 'programming'; title = 'A+B'; tags = @('basic'); statementMd = 'Read two integers, print their sum.';
    bodyJson = @{
      inputFormat = 'two integers a b'; outputFormat = 'one integer a+b';
      samples = @(@{ input = '1 2'; output = '3' });
      testCases = @(
        @{ input = '1 2'; output = '3' },
        @{ input = '100 200'; output = '300' },
        @{ input = '-5 7'; output = '2' })
    };
    answerJson = @{}; solutions = @(); timeLimitMs = 2000; memoryLimitMiB = 256
  }
  $r = JsonOf (Api 'POST' "$mainBase/api/problems" $p $mAdmin)
  if (-not $r -or -not $r.problem) { throw 'create A+B failed' }
  $probId = [long]$r.problem.id
  Ok "main: created A+B problem id=$probId"

  $tf = @{ type = 'true_false'; title = 'TF1'; tags = @('basic'); statementMd = '1+1==2 ?';
          bodyJson = @{}; answerJson = @{ answer = $true }; solutions = @() }
  $r = JsonOf (Api 'POST' "$mainBase/api/problems" $tf $mAdmin)
  if (-not $r -or -not $r.problem) { throw 'create TF failed' }
  $tfId = [long]$r.problem.id
  Ok "main: created true/false problem id=$tfId"

  $r = JsonOf (Api 'POST' "$mainBase/api/trainings" @{ title = 'E2E Training'; description = ''; tags = @() } $mAdmin)
  if (-not $r) { throw 'create training failed' }
  $trId = [long]$r.id
  $r = JsonOf (Api 'POST' "$mainBase/api/trainings/$trId/chapters" @{ title = 'Chapter 1' } $mAdmin)
  if (-not $r) { throw 'create chapter failed' }
  $chId = [long]$r.id
  $r = Api 'POST' "$mainBase/api/chapters/$chId/items" @{ problemIds = @($probId, $tfId) } $mAdmin
  if ($script:lastStatus -ne 200 -and $script:lastStatus -ne 201) { throw 'add chapter items failed' }
  Ok "main: training id=$trId with 2 problems"

  # --- quiz admin: student + assignment (targeted) ---
  $qAdmin = Login $quizBase 'admin' '123456'
  if (-not $qAdmin) { throw 'quiz admin login failed' }
  Ok 'quiz/OJ admin login (shared account)'

  $r = JsonOf (Api 'POST' "$quizBase/api/admin/students" @{ username = 'tester01'; password = 'pw' } $qAdmin)
  if (-not $r) { throw 'create student failed' }
  $stuId = [long]$r.id

  $r = JsonOf (Api 'POST' "$quizBase/api/admin/assignments" @{
    kind = 'training'; repoId = $trId; assignedAll = $false; studentIds = @($stuId) } $qAdmin)
  if (-not $r) { throw 'create assignment failed' }
  $assignId = [long]$r.id
  Ok "quiz: student id=$stuId arranged training (assignment id=$assignId)"

  $r = Api 'POST' "$quizBase/api/admin/assignments" @{
    kind = 'training'; repoId = $trId; assignedAll = $true } $qAdmin
  if ($script:lastStatus -eq 409) { Ok 'duplicate assignment rejected (409)' } else { Bad 'duplicate assignment should 409' }

  # --- student: visibility + task list + training detail ---
  $stu = Login $quizBase 'tester01' 'pw'
  if (-not $stu) { throw 'student login failed' }
  Ok 'student login'

  $r = JsonOf (Api 'GET' "$quizBase/api/oj/assigned" $null $stu)
  if ($r -and $r.trainings.Count -eq 1 -and $r.practices.Count -eq 0) { Ok 'student sees the arranged training only' }
  else { Bad "student assigned list = trainings:$($r.trainings.Count) practices:$($r.practices.Count)" }

  $r = JsonOf (Api 'GET' "$quizBase/api/oj/training/$assignId" $null $stu)
  if ($r -and $r.chapters.Count -eq 1 -and $r.chapters[0].items.Count -eq 2) { Ok 'training detail has 1 chapter x 2 items' }
  else { Bad "training detail unexpected: $($r | ConvertTo-Json -Compress)" }

  # secret not leaked: programming bodyJson must not contain testCases
  $r = JsonOf (Api 'GET' "$quizBase/api/oj/problem/$probId" $null $stu)
  $leak = $false
  if ($r) { foreach ($n in $r.bodyJson.PSObject.Properties.Name) { if ($n -like 'testCase*') { $leak = $true } } }
  if ($r -and -not $leak) { Ok 'problem body has NO testCases (judge keys not leaked)' }
  else { Bad 'programming problem body leaked testCases!' }

  # student cannot reach admin APIs (expect 403)
  $null = Api 'GET' "$quizBase/api/admin/assignments" $null $stu
  if ($script:lastStatus -eq 403) { Ok 'student -> admin API returns 403' } else { Bad "student -> admin API status=$script:lastStatus want 403" }

  # --- objective question: instant verdict + progress ---
  $r = JsonOf (Api 'POST' "$quizBase/api/oj/problem/$tfId/objective-submit" @{ answer = $false } $stu)
  if ($r -and $r.verdict -eq 'WA') { Ok 'objective wrong answer -> WA (score 0)' } else { Bad "objective wrong: $($r | ConvertTo-Json -Compress)" }
  $r = JsonOf (Api 'POST' "$quizBase/api/oj/problem/$tfId/objective-submit" @{ answer = $true } $stu)
  if ($r -and $r.verdict -eq 'AC' -and $r.score -eq 100) { Ok 'objective right answer -> AC (score 100)' } else { Bad "objective right: $($r | ConvertTo-Json -Compress)" }

  # --- programming: REAL python judge ---
  if (-not $pyPath) {
    Skip 'PYTHON assertions (no real python interpreter found on PATH)'
  } else {
    $pyCode = @'
a, b = map(int, input().split())
print(a + b)
'@
    $r = JsonOf (Api 'POST' "$quizBase/api/oj/problem/$probId/submit" @{ language = 'python'; sourceCode = $pyCode } $stu)
    if ($r) {
      $snap = Poll-Judge $quizBase $stu $r.submissionId
      if ($snap -and $snap.verdict -eq 'AC' -and $snap.score -eq 100) { Ok "PYTHON submit -> AC score=100 ($($snap.caseDetails.Count) cases)" }
      else { Bad "PYTHON submit verdict=$($snap.verdict) stderr=$($snap.stderr)" }
    } else { Bad "python submit HTTP $script:lastStatus $script:lastErr" }

    $r = JsonOf (Api 'POST' "$quizBase/api/oj/problem/$probId/run" @{ language = 'python'; sourceCode = $pyCode; inputData = '8 9' } $stu)
    if ($r) {
      $snap = Poll-Judge $quizBase $stu $r.submissionId
      if ($snap -and $snap.verdict -eq 'OK' -and $snap.stdout -match '17') { Ok 'PYTHON run (input 8 9) -> OK, stdout contains 17' }
      else { Bad "PYTHON run verdict=$($snap.verdict) stdout=$($snap.stdout)" }
    } else { Bad "python run HTTP $script:lastStatus $script:lastErr" }

    $pyWrong = @'
a, b = map(int, input().split())
print(a - b)
'@
    $r = JsonOf (Api 'POST' "$quizBase/api/oj/problem/$probId/submit" @{ language = 'python'; sourceCode = $pyWrong } $stu)
    if ($r) {
      $snap = Poll-Judge $quizBase $stu $r.submissionId
      if ($snap -and $snap.verdict -eq 'WA') { Ok 'PYTHON wrong program -> WA' } else { Bad "PYTHON wrong verdict=$($snap.verdict)" }
    } else { Bad "python wrong HTTP $script:lastStatus $script:lastErr" }
  }

  # --- C++ judge (conditional on local g++) ---
  if ($gppPath) {
    $cppCode = @'
#include <iostream>
int main(){ long long a,b; std::cin>>a>>b; std::cout<<a+b<<"\n"; return 0; }
'@
    $r = JsonOf (Api 'POST' "$quizBase/api/oj/problem/$probId/submit" @{ language = 'cpp'; sourceCode = $cppCode } $stu)
    if ($r) {
      $snap = Poll-Judge $quizBase $stu $r.submissionId
      if ($snap -and $snap.verdict -eq 'AC' -and $snap.score -eq 100) { Ok 'CPP submit (real g++ compile+run) -> AC score=100' }
      else { Bad "CPP submit verdict=$($snap.verdict) stderr=$($snap.stderr)" }
    } else { Bad "cpp submit HTTP $script:lastStatus $script:lastErr" }
  } else {
    Skip 'CPP assertions (no local g++ toolchain found)'
  }

  # --- progress / history / stats ---
  # Objective problems (TF) were answered regardless of toolchains; programming AC only when python ran.
  $r = JsonOf (Api 'GET' "$quizBase/api/oj/assigned" $null $stu)
  if ($r) {
    $tr = $r.trainings[0]
    $expectCompleted = 1  # TF answered
    if ($pyPath) { $expectCompleted = 2 }  # + A+B AC via real python
    if ($tr.accepted -eq $expectCompleted -and $tr.accepted -le $tr.problemCount) {
      Ok "student progress completed $($tr.accepted)/$($tr.problemCount) (expected $expectCompleted)"
    } else { Bad "progress = $($r | ConvertTo-Json -Compress)" }
  } else { Bad 'assigned list fetch failed' }

  if ($pyPath) {
    $r = JsonOf (Api 'GET' "$quizBase/api/oj/problem/$probId/submissions" $null $stu)
    if ($r -and $r.submissions.Count -ge 4) { Ok "submission history has $($r.submissions.Count) records" }
    else { Bad "history count = $($r.submissions.Count)" }
  } else {
    Skip 'submission history assertions (python skipped)'
  }

  $r = JsonOf (Api 'GET' "$quizBase/api/admin/assignments/$assignId/stats" $null $qAdmin)
  if ($r -and $r.problems.Count -eq 2) {
    $prog = $r.problems | Where-Object { $_.problemId -eq $probId } | Select-Object -First 1
    $tfStat = $r.problems | Where-Object { $_.problemId -eq $tfId } | Select-Object -First 1
    $statsOk = $true
    if (-not $tfStat -or $tfStat.accepted -lt 1) { $statsOk = $false }               # TF AC by objective
    if ($pyPath -and (-not $prog -or $prog.accepted -lt 1 -or $prog.submissions -lt 2)) { $statsOk = $false }
    if ($r.totalStudents -ne 1) { $statsOk = $false }
    if ($statsOk) { Ok 'admin stats: per-problem accepted/submissions correct (1 targeted student)' }
    else { Bad "admin stats unexpected: $($r | ConvertTo-Json -Compress -Depth 5)" }
  } else { Bad "admin stats problems=$($r.problems.Count)" }

  Write-Host ''
  Write-Host '=== E2E flow finished ===' -ForegroundColor Cyan

} catch {
  Bad "E2E aborted: $($_.Exception.Message)"
  foreach ($log in @((Join-Path $env:TEMP 'orangeoj-test-main.log'), (Join-Path $env:TEMP 'orangeoj-test-judge.log'), (Join-Path $env:TEMP 'orangeoj-test-quiz.log'))) {
    if (Test-Path $log) { Write-Host "--- tail of $log ---" -ForegroundColor DarkYellow; Get-Content $log -Tail 12 }
    if (Test-Path "$log.err") { Get-Content "$log.err" -Tail 12 }
  }
} finally {
  Cleanup
}

Write-Host ''
Write-Host "Result: PASS=$($script:pass) FAIL=$($script:fail) SKIP=$($script:skip)" -ForegroundColor Cyan
if ($script:fail -gt 0) { Write-Host '==> Some checks FAILED (see [FAIL] lines above)' -ForegroundColor Red; exit 1 }
Write-Host '==> All checks passed' -ForegroundColor Green
exit 0
