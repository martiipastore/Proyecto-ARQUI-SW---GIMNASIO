param([string]$RepoRoot=(Get-Location).Path)
$ErrorActionPreference='Stop'
$previousCache=$env:GOCACHE
$previousRequired=$env:REQUIRE_DOCKER_TESTS
$temporary=Join-Path ([IO.Path]::GetTempPath()) ('cb02-tests-'+[guid]::NewGuid().ToString('N')+'.jsonl')
$reportPath=Join-Path $RepoRoot 'docs/evidence/club/cb02-tests.json'
$started=[DateTime]::UtcNow.ToString('o')
Push-Location (Join-Path $RepoRoot 'services/benefits-service')
try {
    $env:GOCACHE=Join-Path ([IO.Path]::GetTempPath()) 'gym-club-go-cache'
    $env:REQUIRE_DOCKER_TESTS='1'
    go test ./... -count=1 -json | Out-File -LiteralPath $temporary -Encoding UTF8
    $testExit=$LASTEXITCODE
    $events=@(Get-Content -LiteralPath $temporary -Encoding UTF8 | Where-Object { $_.StartsWith('{') } | ForEach-Object { $_ | ConvertFrom-Json })
    $tests=@($events | Where-Object { $_.Test -and $_.Action -in @('pass','fail','skip') } | Select-Object Package,Test,Action,Elapsed)
    $packages=@($events | Where-Object { -not $_.Test -and $_.Action -in @('pass','fail','skip') } | Select-Object Package,Action,Elapsed)
    $failed=@($tests | Where-Object Action -eq 'fail')
    $skipped=@($tests | Where-Object Action -eq 'skip')
    $integration=@($tests | Where-Object { $_.Test -eq 'TestLedger_PostgreSQLReal' -and $_.Action -eq 'pass' })
    $passed=$testExit -eq 0 -and $failed.Count -eq 0 -and $skipped.Count -eq 0 -and $integration.Count -eq 1
    $report=[ordered]@{stage='CB-02';startedAtUtc=$started;finishedAtUtc=[DateTime]::UtcNow.ToString('o');command='REQUIRE_DOCKER_TESTS=1 go test ./... -count=1 -json';goExitCode=$testExit;passed=$passed;passedTests=@($tests | Where-Object Action -eq 'pass').Count;failedTests=$failed.Count;skippedTests=$skipped.Count;packages=$packages;tests=$tests;scope='Benefits unit/API/PostgreSQL isolated tests; not full Club E2E, SMTP, deployment, or performance acceptance'}
    [void][IO.Directory]::CreateDirectory((Split-Path $reportPath))
    [IO.File]::WriteAllText($reportPath,($report | ConvertTo-Json -Depth 8),[Text.UTF8Encoding]::new($false))
    if(-not $passed) { throw "CB-02 validation failed; report: $reportPath. Local diagnostic output: $temporary" }
    Write-Output "CB-02: $($report.passedTests) test/subtest results passed; zero skips. Report: $reportPath"
} finally {
    $env:GOCACHE=$previousCache
    $env:REQUIRE_DOCKER_TESTS=$previousRequired
    Pop-Location
}
