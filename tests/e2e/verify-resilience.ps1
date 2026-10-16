param(
    [string]$BaseUrl = 'http://localhost:8080',
    [string]$BenefitsUrl = 'http://localhost:8083',
    [string]$JaegerUrl = 'http://localhost:16686',
    [string]$EvidenceDirectory = 'docs/evidence/etapa6'
)
# D10 / HU-29 / HU-32 / RN-22 / RN-26 / RN-27 / CL-08 y CL-08b.
# Solo para el gimnasio local: detiene Benefits y pausa su PostgreSQL temporalmente.
$ErrorActionPreference = 'Stop'
$OutputEncoding = [System.Text.UTF8Encoding]::new($false)
Add-Type -AssemblyName System.Net.Http
Push-Location (Resolve-Path (Join-Path $PSScriptRoot '../..'))
$client = [System.Net.Http.HttpClient]::new()
$client.Timeout = [TimeSpan]::FromSeconds(10)
$baseCompose = @('compose', '--env-file', '.env', '-f', 'deploy/docker-compose.yml')
$faultCompose = $baseCompose + @('-f', 'tests/resilience/docker-compose.yml')
$timeline = [Collections.Generic.List[object]]::new()
$attempts = [Collections.Generic.List[object]]::new()
$tracesToExport = [Collections.Generic.List[string]]::new()
$capturedLogs = [Collections.Generic.List[object]]::new()
$stateChecks = [Collections.Generic.List[object]]::new()
$pausedByScript = $false
$restoreRequired = $false
$previousMode = $env:RESILIENCE_FAULT_MODE

function Record([string]$Action) {
    $timeline.Add([pscustomobject]@{ utc = [DateTime]::UtcNow.ToString('o'); action = $Action })
}
function Compose([string[]]$Arguments, [switch]$Fault) {
    $prefix = if ($Fault) { $faultCompose } else { $baseCompose }
    & docker @prefix @Arguments | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Falló un comando de Compose del ejercicio.' }
}
function SQL([string]$Query) {
    $result = $Query | & docker @baseCompose exec -T postgres sh -c 'PGPASSWORD="$BENEFITS_DB_PASSWORD" exec psql -Atq -v ON_ERROR_STOP=1 -U benefits_user -d benefits_db'
    if ($LASTEXITCODE -ne 0) { throw 'Falló la consulta del fixture sintético.' }
    return ($result -join "`n")
}
function Snapshot([int]$Expected) {
    $query = "SELECT json_build_object('balance',saldo,'movements',(SELECT count(*) FROM movimientos_puntos WHERE cuenta_id=c.id),'operations',(SELECT count(*) FROM operaciones_partner WHERE partner_id='$partner')) FROM cuentas c WHERE id='$account';"
    $state = SQL $query | ConvertFrom-Json
    if ($state.balance -ne (100 * $Expected) -or $state.movements -ne $Expected -or $state.operations -ne $Expected) { throw 'El estado durable no coincide con lo esperado.' }
    $stateChecks.Add([pscustomobject]@{ utc = [DateTime]::UtcNow.ToString('o'); expected_movements = $Expected; balance = $state.balance; movements = $state.movements; operations = $state.operations })
    return $state
}
function Hash-Body([string]$Text) {
    $sha = [Security.Cryptography.SHA256]::Create()
    try { return [BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($Text))).Replace('-', '').ToLowerInvariant() } finally { $sha.Dispose() }
}
function Credit([string]$Scenario, [string]$Key) {
    $request = [System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::Post, $BaseUrl + '/partner-api/v1/accounts/' + $externalUser + '/credits')
    $traceId = [guid]::NewGuid().ToString('N')
    $correlation = [guid]::NewGuid().ToString()
    try {
        $request.Headers.Add('X-API-Key', $apiKey)
        $request.Headers.Add('Idempotency-Key', $Key)
        $request.Headers.Add('X-Correlation-ID', $correlation)
        $request.Headers.Add('traceparent', '00-' + $traceId + '-0123456789abcdef-01')
        $request.Content = [System.Net.Http.StringContent]::new('{"points":100,"reason":"Prueba de resiliencia","reference":"resilience-fixture"}', [Text.Encoding]::UTF8, 'application/json')
        $started = [DateTime]::UtcNow.ToString('o')
        $clock = [Diagnostics.Stopwatch]::StartNew()
        $response = $client.SendAsync($request).GetAwaiter().GetResult()
        try {
            $text = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
            $clock.Stop()
            $data = $text | ConvertFrom-Json
            $replayed = $response.Headers.Contains('Idempotent-Replayed') -and ($response.Headers.GetValues('Idempotent-Replayed') -contains 'true')
            $retryAfter = if ($response.Headers.Contains('Retry-After')) { @($response.Headers.GetValues('Retry-After')) -join ',' } else { '' }
            $attempt = [pscustomobject]@{ scenario = $Scenario; utc = $started; status = [int]$response.StatusCode; code = $data.code; duration_ms = [Math]::Round($clock.Elapsed.TotalMilliseconds, 3); trace_id = $traceId; correlation_id = $correlation; replayed = $replayed; response_sha256 = (Hash-Body $text); retry_after = $retryAfter }
            $attempts.Add($attempt)
            if ($attempt.status -ge 400) {
                if ($response.Content.Headers.ContentType.MediaType -ne 'application/problem+json' -or $data.correlationId -ne $correlation) { throw 'El error no respeta el contrato público de partners.' }
            }
            return $attempt
        } finally { $response.Dispose() }
    } finally { $request.Dispose() }
}
function Capture-Logs($Attempt) {
    # Guardar antes de recrear el gateway: Docker elimina logs del contenedor anterior.
    $lines = & docker @baseCompose logs --no-color --no-log-prefix --since 15m api-gateway benefits-service
    if ($LASTEXITCODE -ne 0) { throw 'No se pudieron recuperar logs del intento.' }
    foreach ($line in $lines) {
        try { $entry = $line | ConvertFrom-Json } catch { continue }
        if ($entry.trace_id -eq $Attempt.trace_id) {
            if ($entry.correlation_id -ne $Attempt.correlation_id -or -not $entry.span_id) { throw 'Un log perdió su contexto de correlación.' }
            $capturedLogs.Add($entry)
        }
    }
}
function Assert-Failure($Attempt, [int]$Status, [string]$Code, [int]$BudgetMs) {
    if ($Attempt.status -ne $Status -or $Attempt.code -ne $Code) { throw ('Respuesta inesperada en ' + $Attempt.scenario) }
    # D-56: tolerancia de medición aprobada; no modifica los timeouts de código.
    if ($Attempt.duration_ms -gt ($BudgetMs + 500)) { throw ('Se excedió el presupuesto con tolerancia en ' + $Attempt.scenario) }
    if ($Status -ne 504 -and -not $Attempt.retry_after) { throw 'Falta Retry-After contractual.' }
    $tracesToExport.Add($Attempt.trace_id)
    Capture-Logs $Attempt
}
function Recover([string]$Scenario, [string]$Key, [int]$Expected) {
    $first = Credit ($Scenario + '_recovery') $Key
    $replay = Credit ($Scenario + '_replay') $Key
    if ($first.status -ne 201 -or $replay.status -ne 201 -or -not $replay.replayed -or $first.response_sha256 -ne $replay.response_sha256) { throw 'La recuperación no conserva el resultado idempotente.' }
    $null = Snapshot $Expected
}
function Wait-Ready([string]$Url) {
    $deadline = [DateTime]::UtcNow.AddSeconds(60)
    do {
        try { $r = $client.GetAsync($Url + '/health/ready').GetAwaiter().GetResult(); try { if ($r.IsSuccessStatusCode) { return } } finally { $r.Dispose() } } catch { }
        Start-Sleep -Milliseconds 1000
    } while ([DateTime]::UtcNow -lt $deadline)
    throw 'El servicio no recuperó readiness.'
}

try {
    Wait-Ready $BenefitsUrl
    Wait-Ready $BaseUrl
    # Rechazar un entorno previamente pausado; nunca asumir que es nuestro cambio.
    $pgId = (& docker @baseCompose ps -q postgres).Trim()
    if (-not $pgId -or (& docker inspect --format '{{.State.Paused}}' $pgId) -ne 'false') { throw 'PostgreSQL no está disponible para iniciar el ejercicio.' }
    $partner = [guid]::NewGuid().ToString()
    $account = [guid]::NewGuid().ToString()
    $student = [guid]::NewGuid().ToString()
    $externalUser = 'resilience-' + [guid]::NewGuid().ToString('N')
    $random = [Security.Cryptography.RandomNumberGenerator]::Create()
    $bytes = New-Object byte[] 32
    try { $random.GetBytes($bytes) } finally { $random.Dispose() }
    $apiKey = [BitConverter]::ToString($bytes).Replace('-', '').ToLowerInvariant()
    $credentialHash = Hash-Body $apiKey
    $keys = @(1..4 | ForEach-Object { [guid]::NewGuid().ToString() })
    $seed = "BEGIN; INSERT INTO partners(id,nombre,api_key_hash) VALUES ('$partner','Fixture resiliencia',decode('$credentialHash','hex')); INSERT INTO cuentas(id,alumno_id) VALUES ('$account','$student'); INSERT INTO vinculaciones(partner_id,external_user_id,cuenta_id) VALUES ('$partner','$externalUser','$account'); COMMIT;"
    $null = SQL $seed
    $null = Snapshot 0
    $restoreRequired = $true

    Record 'stop_benefits'
    Compose @('stop', 'benefits-service')
    $down = Credit 'benefits_stopped' $keys[0]
    Assert-Failure $down 502 'UNAVAILABLE' 5000
    $null = Snapshot 0
    Compose @('up', '-d', '--no-deps', '--wait', 'benefits-service')
    Record 'benefits_restored'
    Recover 'benefits_stopped' $keys[0] 1

    Record 'pause_postgres'
    $pausedByScript = $true
    Compose @('pause', 'postgres')
    try {
        $live = $client.GetAsync($BenefitsUrl + '/health/live').GetAwaiter().GetResult()
        $ready = $client.GetAsync($BenefitsUrl + '/health/ready').GetAwaiter().GetResult()
        try { if ([int]$live.StatusCode -ne 200 -or [int]$ready.StatusCode -ne 503) { throw 'Liveness/readiness no reflejan la dependencia crítica.' } } finally { $live.Dispose(); $ready.Dispose() }
        $database = Credit 'postgres_paused' $keys[1]
        Assert-Failure $database 503 'UNAVAILABLE' 4000
    } finally { Compose @('unpause', 'postgres'); $pausedByScript = $false }
    Wait-Ready $BenefitsUrl
    Record 'postgres_restored'
    $null = Snapshot 1
    Recover 'postgres_paused' $keys[1] 2

    $env:RESILIENCE_FAULT_MODE = 'before'
    Record 'inject_delay_before_benefits'
    Compose @('up', '-d', '--build', '--no-deps', '--wait', 'resilience-proxy', 'api-gateway') -Fault
    $before = Credit 'timeout_before_commit' $keys[2]
    Assert-Failure $before 504 'TIMEOUT' 5000
    $null = Snapshot 2
    Compose @('up', '-d', '--no-deps', '--wait', 'api-gateway')
    Record 'restore_direct_route'
    Recover 'timeout_before_commit' $keys[2] 3

    $env:RESILIENCE_FAULT_MODE = 'after-once'
    Record 'inject_response_loss_after_commit'
    Compose @('up', '-d', '--no-deps', '--force-recreate', '--wait', 'resilience-proxy', 'api-gateway') -Fault
    $after = Credit 'timeout_after_commit' $keys[3]
    Assert-Failure $after 504 'TIMEOUT' 5000
    $null = Snapshot 4
    $proxyLogs = & docker @faultCompose logs --no-color --no-log-prefix resilience-proxy
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo recuperar la evidencia del inyector.' }
    $withheld = @($proxyLogs | ForEach-Object { try { $e = $_ | ConvertFrom-Json; if ($e.event -eq 'response_withheld' -and $e.trace_id -eq $after.trace_id) { $e } } catch { } })
    if ($withheld.Count -ne 1 -or $withheld[0].original_status -ne 201) { throw 'No se comprobó la respuesta confirmada antes de perderla.' }
    $retry = Credit 'timeout_after_commit_recovery' $keys[3]
    if ($retry.status -ne 201 -or -not $retry.replayed -or $retry.response_sha256 -ne $withheld[0].response_sha256) { throw 'El reintento no recuperó exactamente la respuesta confirmada.' }
    $tracesToExport.Add($retry.trace_id)
    Capture-Logs $retry
    $finalState = Snapshot 4
    Record 'recovered_without_duplicates'
} finally {
    try {
        if ($pausedByScript) { Compose @('unpause', 'postgres') }
        if ($restoreRequired) {
            Compose @('up', '-d', '--no-deps', '--wait', 'benefits-service', 'api-gateway')
            Compose @('rm', '--stop', '--force', 'resilience-proxy') -Fault
            Wait-Ready $BenefitsUrl
            Wait-Ready $BaseUrl
            Record 'restored_normal_stack'
        }
    } finally {
        if ($null -eq $previousMode) { Remove-Item Env:RESILIENCE_FAULT_MODE -ErrorAction SilentlyContinue } else { $env:RESILIENCE_FAULT_MODE = $previousMode }
        $client.Dispose()
        Pop-Location
    }
}

# No exportar evidencia de éxito hasta haber restaurado el entorno.
$exportedTraces = @()
foreach ($traceId in $tracesToExport) {
    $deadline = [DateTime]::UtcNow.AddSeconds(60)
    $trace = $null
    $scenario = @($attempts | Where-Object { $_.trace_id -eq $traceId })[0].scenario
    $minimumSpans = switch ($scenario) { "postgres_paused" { 4 } "timeout_after_commit" { 5 } "timeout_after_commit_recovery" { 5 } default { 2 } }
    do {
        try { $trace = Invoke-RestMethod ($JaegerUrl + '/api/traces/' + $traceId) } catch { $trace = $null }
        if ($trace -and $trace.data -and $trace.data[0].spans.Count -ge $minimumSpans) { break }
        Start-Sleep -Milliseconds 1000
    } while ([DateTime]::UtcNow -lt $deadline)
    if (-not $trace -or -not $trace.data -or $trace.data[0].spans.Count -lt $minimumSpans) { throw 'Falta una traza completa del ejercicio.' }
    $exportedTraces += $trace.data[0]
}
Push-Location (Resolve-Path (Join-Path $PSScriptRoot '../..'))
try {
    $logs = $capturedLogs.ToArray()
    foreach ($id in $tracesToExport) {
        if (@($logs | Where-Object { $_.trace_id -eq $id -and $_.service -eq 'api-gateway' }).Count -lt 1) { throw 'Falta el log del gateway para una falla o recuperación.' }
    }
    $summary = [pscustomobject]@{ measured_at_utc = [DateTime]::UtcNow.ToString('o'); tolerance_ms = 500; benefits_budget_ms = 4000; gateway_budget_ms = 5000; attempts = $attempts.ToArray(); timeline = $timeline.ToArray(); state_checks = $stateChecks.ToArray(); final_state = $finalState; confirmed_response = $withheld[0]; restored = $true }
    $summaryJson = $summary | ConvertTo-Json -Depth 100
    $tracesJson = [pscustomobject]@{ data = $exportedTraces } | ConvertTo-Json -Depth 100
    $logsJson = ($logs | ForEach-Object { $_ | ConvertTo-Json -Compress -Depth 30 }) -join "`n"
    foreach ($sensitive in (@($apiKey, $credentialHash, $partner, $account, $student, $externalUser, 'Prueba de resiliencia', 'resilience-fixture') + $keys)) {
        if ($summaryJson.Contains($sensitive) -or $tracesJson.Contains($sensitive) -or $logsJson.Contains($sensitive)) { throw 'La evidencia contiene datos excluidos.' }
    }
    New-Item -ItemType Directory -Force -Path $EvidenceDirectory | Out-Null
    foreach ($item in @(@('results.json', $summaryJson), @('traces.json', $tracesJson), @('logs.jsonl', $logsJson + "`n"))) {
        [IO.File]::WriteAllText((Join-Path (Resolve-Path $EvidenceDirectory) $item[0]), $item[1], [Text.UTF8Encoding]::new($false))
    }
    Write-Output 'Resiliencia: OK (502, 503, 504 antes y después de commit; saldo 400, 4 movimientos, 4 operaciones; entorno restaurado).'
    Write-Output ('Evidencia sanitizada en ' + $EvidenceDirectory)
} finally { Pop-Location }
