param([string]$RepoRoot=(Get-Location).Path)
$ErrorActionPreference='Stop'
$previousCache=$env:GOCACHE; $previousRequired=$env:REQUIRE_DOCKER_TESTS
$target=Join-Path $RepoRoot 'docs/evidence/club/cb04-tests.json'
$report=[ordered]@{stage='CB-04';startedAtUtc=[DateTime]::UtcNow.ToString('o');finishedAtUtc=$null;passed=$false;modules=@();scope='Membership, HTTP, PostgreSQL and vet for gateway/members; isolated cross-service E2E is separate in cb04-e2e.json'}
[void][IO.Directory]::CreateDirectory((Split-Path $target))
function Save-Report { [IO.File]::WriteAllText($target,($report | ConvertTo-Json -Depth 10),[Text.UTF8Encoding]::new($false)) }
Save-Report
try {
    $env:GOCACHE=Join-Path ([IO.Path]::GetTempPath()) 'gym-club-go-cache'
    $env:REQUIRE_DOCKER_TESTS='1'
    foreach($module in @('gateway','services/members-service')) {
        $diagnostic=Join-Path ([IO.Path]::GetTempPath()) ('cb04-tests-'+[guid]::NewGuid().ToString('N')+'.jsonl')
        Push-Location (Join-Path $RepoRoot $module)
        try {
            go test ./... -count=1 -json | Out-File -LiteralPath $diagnostic -Encoding UTF8
            $testExit=$LASTEXITCODE
            go vet ./...
            $vetExit=$LASTEXITCODE
        } finally { Pop-Location }
        $events=@(Get-Content -LiteralPath $diagnostic -Encoding UTF8 | Where-Object { $_.StartsWith('{') } | ForEach-Object { $_|ConvertFrom-Json })
        $tests=@($events|Where-Object { $_.Test -and $_.Action -in @('pass','fail','skip') }|Select-Object Package,Test,Action,Elapsed)
        $failures=@($tests|Where-Object Action -eq 'fail').Count
        $skips=@($tests|Where-Object Action -eq 'skip').Count
        $integrationRequired=$module -eq 'services/members-service'
        $integrationPassed=@($tests|Where-Object { $_.Test -eq 'TestMembresiasPostgres' -and $_.Action -eq 'pass' }).Count -eq 1
        $passed=$testExit -eq 0 -and $vetExit -eq 0 -and $failures -eq 0 -and $skips -eq 0 -and (-not $integrationRequired -or $integrationPassed)
        $report.modules+=@{module=$module;testExitCode=$testExit;vetExitCode=$vetExit;passed=$passed;passedTests=@($tests|Where-Object Action -eq 'pass').Count;failedTests=$failures;skippedTests=$skips;tests=$tests}
        Save-Report
        if(-not $passed) { throw "CB-04 failed in $module. Local diagnostic: $diagnostic" }
        Write-Output "CB-04 passed: $module (tests, zero skips, vet)"
    }
    $report.passed=$true
} finally {
    $report.finishedAtUtc=[DateTime]::UtcNow.ToString('o'); Save-Report
    $env:GOCACHE=$previousCache; $env:REQUIRE_DOCKER_TESTS=$previousRequired
}
