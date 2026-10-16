param(
    [ValidateSet('read','write','replay','mixed')][string[]]$Workloads = @('read','write','replay','mixed'),
    [ValidateSet('hot','distributed')][string[]]$Distributions = @('hot','distributed'),
    [string]$EvidenceDirectory = 'docs/evidence/etapa7',
    [switch]$Smoke,
    [switch]$Probe
)
# D13 / RN-22 / RN-26 / RN-27. Solo datos sinteticos en el stack local.
$ErrorActionPreference = 'Stop'
if ($Probe -and $Smoke) { throw 'Probe y Smoke son modos distintos.' }
if ($Probe) {
    $Workloads = @('write')
    $Distributions = @('hot')
    if (-not $PSBoundParameters.ContainsKey('EvidenceDirectory')) { $EvidenceDirectory = 'docs/evidence/etapa7/probe' }
}
$pauseMs = $(if ($Probe) { 0 } else { 100 })
$OutputEncoding = [Text.UTF8Encoding]::new($false)
Add-Type -AssemblyName System.Net.Http
Push-Location (Resolve-Path (Join-Path $PSScriptRoot '../..'))
$compose = @('compose','--env-file','.env','-f','deploy/docker-compose.yml')
$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$taskTemp = Join-Path $tempRoot ('gym-load-' + [guid]::NewGuid().ToString('N'))
$oldKey = $env:GYM_LOAD_API_KEY
$activeContainer = $null
$partners = [Collections.Generic.List[string]]::new()
$client = [Net.Http.HttpClient]::new()
$client.Timeout = [TimeSpan]::FromSeconds(10)

function SQL([string]$Text) {
    $output = $Text | & docker @compose exec -T postgres sh -c 'PGPASSWORD="$BENEFITS_DB_PASSWORD" exec psql -Atq -v ON_ERROR_STOP=1 -U benefits_user -d benefits_db'
    if ($LASTEXITCODE -ne 0) { throw 'Fallo de SQL del ejercicio de carga.' }
    return $output -join "`n"
}
function Hash([string]$Text) {
    $sha = [Security.Cryptography.SHA256]::Create()
    try { return [BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($Text))).Replace('-','').ToLowerInvariant() } finally { $sha.Dispose() }
}
function Bytes([string]$Text) {
    if ($Text.Trim() -notmatch '^([\d.]+)\s*([KMGT]?i?B)$') { return 0 }
    $number = [double]::Parse($Matches[1], [Globalization.CultureInfo]::InvariantCulture)
    $factor = switch ($Matches[2]) { 'KiB' { 1024 } 'MiB' { 1048576 } 'GiB' { 1073741824 } 'TiB' { 1099511627776 } 'kB' { 1000 } 'MB' { 1000000 } 'GB' { 1000000000 } default { 1 } }
    return [long]($number * $factor)
}
function Sample {
    $raw = & docker stats --no-stream --format '{{json .}}'
    if ($LASTEXITCODE -ne 0) { throw 'No se pudieron medir los contenedores.' }
    $services = @()
    $otherCpu = 0.0
    $otherMemory = 0L
    foreach ($line in $raw) {
        $s = $line | ConvertFrom-Json
        $cpu = [double]::Parse($s.CPUPerc.TrimEnd('%'), [Globalization.CultureInfo]::InvariantCulture)
        $memory = Bytes ($s.MemUsage.Split('/')[0])
        $name = switch -Wildcard ($s.Name) {
            'gym-api-gateway-*' { 'gateway' }
            'gym-benefits-service-*' { 'benefits' }
            'gym-postgres-*' { 'postgres' }
            'gym-jaeger-*' { 'jaeger' }
            'gym-otel-collector-*' { 'collector' }
            'gym-load-*' { 'generator' }
            default { 'other' }
        }
        if ($name -eq 'other') { $otherCpu += $cpu; $otherMemory += $memory }
        else { $services += [pscustomobject]@{ service=$name; cpu_percent=$cpu; memory_bytes=$memory } }
    }
    $pg = SQL "SELECT json_build_object('connections',count(*),'active',count(*) FILTER (WHERE state='active'),'lock_waiters',count(*) FILTER (WHERE wait_event_type='Lock'),'io_waiters',count(*) FILTER (WHERE wait_event_type='IO')) FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid();" | ConvertFrom-Json
    return [pscustomobject]@{ utc=[DateTime]::UtcNow.ToString('o'); services=$services; other_cpu_percent=$otherCpu; other_memory_bytes=$otherMemory; postgres=$pg }
}
function Write-Json([string]$Path, $Value) {
    [IO.File]::WriteAllText($Path, ($Value | ConvertTo-Json -Depth 100), [Text.UTF8Encoding]::new($false))
}

try {
    New-Item -ItemType Directory -Path $taskTemp | Out-Null
    New-Item -ItemType Directory -Force -Path $EvidenceDirectory | Out-Null
    $evidence = (Resolve-Path $EvidenceDirectory).Path
    $scripts = (Resolve-Path 'tests/load').Path
    $ready = Invoke-RestMethod 'http://localhost:8080/api/v1/status'
    if ($ready.status -ne 'ready') { throw 'El gimnasio no esta listo para medir.' }
    $dockerEnvironment = (& docker info --format '{{json .}}') | ConvertFrom-Json
    if ($LASTEXITCODE -ne 0) { throw 'Docker no esta disponible.' }
    $environment = [pscustomobject]@{
        measured_at_utc=[DateTime]::UtcNow.ToString('o'); smoke=[bool]$Smoke; probe=[bool]$Probe
        docker_version=$dockerEnvironment.ServerVersion; docker_arch=$dockerEnvironment.Architecture
        docker_cpus=$dockerEnvironment.NCPU; docker_memory_bytes=$dockerEnvironment.MemTotal
        host_cpu=@(Get-CimInstance Win32_Processor | ForEach-Object { $_.Name })
        host_memory_kib=(Get-CimInstance Win32_OperatingSystem).TotalVisibleMemorySize
        k6_image='grafana/k6:1.3.0'; traces_sampling='100 percent, unchanged'
        target='http://api-gateway:8080 through public partner prefix'; generator_network='gym_default'
        pause_ms=$pauseMs; accounts_per_fixture=100; seed_movements_per_account=10
        warmup_seconds=$(if ($Smoke) { 1 } else { 30 }); level_seconds=$(if ($Smoke) { 2 } elseif ($Probe) { 30 } else { 60 })
        vu_levels=$(if ($Smoke) { @(1) } else { @(1,5,10) }); sample_target_interval_seconds=5
        pool_note='pgxpool default max(4, NumCPU) when no pool_max_conns is configured; verify against runtime CPU count'
        postgres_settings=(SQL "SELECT json_build_object('max_connections',current_setting('max_connections'),'shared_buffers',current_setting('shared_buffers'),'version',version());" | ConvertFrom-Json)
    }
    Write-Json (Join-Path $evidence 'environment.json') $environment
    foreach ($workload in $Workloads) {
        foreach ($distribution in $Distributions) {
            $caseName = $workload + '-' + $distribution
            $caseDir = Join-Path $evidence $caseName
            New-Item -ItemType Directory -Force -Path $caseDir | Out-Null
            $partner = [guid]::NewGuid().ToString()
            $partners.Add($partner)
            $random = [Security.Cryptography.RandomNumberGenerator]::Create()
            $keyBytes = New-Object byte[] 32
            try { $random.GetBytes($keyBytes) } finally { $random.Dispose() }
            $apiKey = [BitConverter]::ToString($keyBytes).Replace('-','').ToLowerInvariant()
            $env:GYM_LOAD_API_KEY = $apiKey
            $fixtureId = [guid]::NewGuid().ToString('N')
            $accounts = @()
            $seed = [Text.StringBuilder]::new()
            $null = $seed.Append("BEGIN; INSERT INTO partners(id,nombre,api_key_hash) VALUES ('$partner','Fixture carga',decode('$(Hash $apiKey)','hex'));")
            foreach ($i in 0..99) {
                $account = [guid]::NewGuid().ToString()
                $student = [guid]::NewGuid().ToString()
                $externalUser = 'load-' + [guid]::NewGuid().ToString('N')
                $null = $seed.Append("INSERT INTO cuentas(id,alumno_id) VALUES ('$account','$student'); INSERT INTO vinculaciones(partner_id,external_user_id,cuenta_id) VALUES ('$partner','$externalUser','$account');")
                $accounts += [pscustomobject]@{ externalUser=$externalUser; internalId=$account; seeds=@() }
            }
            $null = $seed.Append('COMMIT;')
            $null = SQL $seed.ToString()
            foreach ($i in 0..99) {
                foreach ($slot in 0..9) {
                    $key = "seed-$fixtureId-$i-$slot"
                    $request = [Net.Http.HttpRequestMessage]::new([Net.Http.HttpMethod]::Post, 'http://localhost:8080/partner-api/v1/accounts/' + $accounts[$i].externalUser + '/credits')
                    try {
                        $request.Headers.Add('X-API-Key',$apiKey)
                        $request.Headers.Add('Idempotency-Key',$key)
                        $request.Content = [Net.Http.StringContent]::new('{"points":1,"reason":"Carga sintetica","reference":"load-fixture"}',[Text.Encoding]::UTF8,'application/json')
                        $response = $client.SendAsync($request).GetAwaiter().GetResult()
                        try {
                            if ([int]$response.StatusCode -ne 201) { throw 'Fallo la precarga del fixture.' }
                            $body = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
                            $accounts[$i].seeds += [pscustomobject]@{ key=$key; hash=(Hash $body) }
                        } finally { $response.Dispose() }
                    } finally { $request.Dispose() }
                }
            }
            $fixtureFile = Join-Path $taskTemp 'fixture.json'
            Write-Json $fixtureFile ([pscustomobject]@{ accounts=$accounts })
            $expectedCredits = 1000L
            $phases = @([pscustomobject]@{ name='warmup'; vus=1; seconds=$environment.warmup_seconds })
            foreach ($vu in $environment.vu_levels) { $phases += [pscustomobject]@{ name="vu$vu"; vus=$vu; seconds=$environment.level_seconds } }
            foreach ($phase in $phases) {
                $phaseId = $fixtureId + '-' + $phase.name
                $output = Join-Path $taskTemp 'output'
                New-Item -ItemType Directory -Force -Path $output | Out-Null
                $summaryPath = Join-Path $output 'summary.json'
                if (Test-Path $summaryPath) { Remove-Item -LiteralPath $summaryPath }
                $activeContainer = 'gym-load-' + [guid]::NewGuid().ToString('N')
                $dockerArgs = @('run','--rm','--name',$activeContainer,'--network','gym_default','--env','GYM_LOAD_API_KEY',
                    '--env',"WORKLOAD=$workload",'--env',"DISTRIBUTION=$distribution",'--env',"VUS=$($phase.vus)",
                    '--env',"DURATION=$($phase.seconds)s",'--env',"PAUSE_MS=$pauseMs",'--env',"PHASE=$($phase.name)",
                    '--env',"RUN_ID=$phaseId",'--env','BASE_URL=http://api-gateway:8080',
                    '-v',"${scripts}:/scripts:ro",'-v',"${fixtureFile}:/input/fixture.json:ro",'-v',"${output}:/output",
                    'grafana/k6:1.3.0','run','--quiet','/scripts/benefits.js')
                # Cada argumento queda entre comillas; los valores se generan localmente.
                if (@($dockerArgs | Where-Object { $_.Contains('"') }).Count) { throw 'Argumento local no representable.' }
                $arguments = ($dockerArgs | ForEach-Object { '"' + $_ + '"' }) -join ' '
                $process = Start-Process -FilePath 'docker.exe' -ArgumentList $arguments -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $taskTemp 'k6.stdout') -RedirectStandardError (Join-Path $taskTemp 'k6.stderr')
                $null = $process.Handle
                $samples = @()
                do {
                    $samples += Sample
                    Start-Sleep -Seconds 5
                    $process.Refresh()
                } while (-not $process.HasExited)
                $process.WaitForExit()
                if (-not (Test-Path $summaryPath)) { throw 'k6 no genero resumen; comprobar imagen, montajes y recursos del entorno.' }
                $summary = Get-Content $summaryPath -Raw -Encoding UTF8 | ConvertFrom-Json
                $exitCode = $process.ExitCode
                if ($null -eq $exitCode) { throw "No se pudo obtener el codigo de salida del proceso." }
                $summary | Add-Member -NotePropertyName exit_code -NotePropertyValue $exitCode
                $activeContainer = $null
                if ($exitCode -ne 0 -and $exitCode -ne 99) { throw ('k6 no completo el escenario; codigo=' + $exitCode) }
                if ($summary.metrics.credits_created) { $expectedCredits += [long]$summary.metrics.credits_created.values.count }
                $state = SQL "SELECT json_build_object('accounts',count(*),'balance_sum',sum(c.saldo),'movements',(SELECT count(*) FROM movimientos_puntos WHERE partner_id='$partner'),'operations',(SELECT count(*) FROM operaciones_partner WHERE partner_id='$partner'),'inconsistent_accounts',count(*) FILTER (WHERE c.saldo<>(SELECT COALESCE(sum(puntos),0) FROM movimientos_puntos WHERE cuenta_id=c.id) OR c.ultima_secuencia<>(SELECT count(*) FROM movimientos_puntos WHERE cuenta_id=c.id))) FROM cuentas c JOIN vinculaciones v ON v.cuenta_id=c.id WHERE v.partner_id='$partner';" | ConvertFrom-Json
                $invariants = $state.accounts -eq 100 -and $state.balance_sum -eq $expectedCredits -and $state.movements -eq $expectedCredits -and $state.operations -eq $expectedCredits -and $state.inconsistent_accounts -eq 0
                $result = [pscustomobject]@{ summary=$summary; state=$state; expected_credits=$expectedCredits; invariants_ok=$invariants; samples=$samples }
                $serialized = $result | ConvertTo-Json -Depth 100
                foreach ($sensitive in @($apiKey,$partner,$fixtureId)) { if ($serialized.Contains($sensitive)) { throw 'El resumen contiene datos excluidos.' } }
                [IO.File]::WriteAllText((Join-Path $caseDir ($phase.name + '.json')), $serialized, [Text.UTF8Encoding]::new($false))
                $failureRate = $summary.metrics.functional_failures.values.rate
                Write-Output ("$caseName $($phase.name): functional_failures=$failureRate, invariants=$invariants, threshold_exit=$exitCode")
                if (-not $invariants -or $failureRate -ne 0) { throw 'Fallo funcional o inconsistencia: se detiene la carga y se conserva la evidencia.' }
            }
            # Plan real del agregado usado por la invariante; no ejecutar cambios de esquema.
            $accountId = $accounts[0].internalId
            $plan = SQL "EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) SELECT COALESCE(sum(puntos),0),count(*),COALESCE(max(secuencia),0) FROM movimientos_puntos WHERE cuenta_id='$accountId';"
            $plan = $plan.Replace($accountId, '<account>')
            [IO.File]::WriteAllText((Join-Path $caseDir 'ledger-plan.json'), $plan, [Text.UTF8Encoding]::new($false))
            $null = SQL "UPDATE partners SET api_key_hash=decode('$(Hash ([guid]::NewGuid().ToString()))','hex') WHERE id='$partner';"
            Write-Output ("Completado: $caseName")
        }
    }
    Write-Output 'Carga finalizada; revisar umbrales de latencia por separado de las invariantes.'
} finally {
    if ($activeContainer) { & docker rm --force $activeContainer | Out-Null }
    foreach ($partner in $partners) {
        try { $null = SQL "UPDATE partners SET api_key_hash=decode('$(Hash ([guid]::NewGuid().ToString()))','hex') WHERE id='$partner';" } catch { Write-Warning 'No se pudo revocar una credencial sintetica.' }
    }
    $env:GYM_LOAD_API_KEY = $oldKey
    $client.Dispose()
    # Verificar el destino antes de borrar recursivamente solo nuestro temporal.
    $resolved = [IO.Path]::GetFullPath($taskTemp)
    if ($resolved.StartsWith($tempRoot,[StringComparison]::OrdinalIgnoreCase) -and [IO.Path]::GetFileName($resolved).StartsWith('gym-load-')) {
        if (Test-Path -LiteralPath $resolved) { Remove-Item -LiteralPath $resolved -Recurse -Force }
    }
    Pop-Location
}
