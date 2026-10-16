param(
 [string]$BaseUrl = 'http://localhost:8080',
 [switch]$Observability,
 [string]$JaegerUrl = 'http://localhost:16686',
 [string]$EvidenceDirectory = 'docs/evidence/etapa4'
)
$ErrorActionPreference = 'Stop'
$OutputEncoding = [System.Text.UTF8Encoding]::new($false)
Add-Type -AssemblyName System.Net.Http
Push-Location (Resolve-Path (Join-Path $PSScriptRoot '../..'))
$client = [System.Net.Http.HttpClient]::new()
$client.Timeout = [TimeSpan]::FromSeconds(10)
try {
    $partner = [guid]::NewGuid().ToString()
    $cuenta = [guid]::NewGuid().ToString()
    $alumno = [guid]::NewGuid().ToString()
    $usuario = 'demo-' + [guid]::NewGuid().ToString('N')
    $bytes = New-Object byte[] 32
    $random = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $random.GetBytes($bytes) } finally { $random.Dispose() }
    $apiKey = [BitConverter]::ToString($bytes).Replace('-', '').ToLowerInvariant()
    $sha = [Security.Cryptography.SHA256]::Create()
    try { $hash = [BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($apiKey))).Replace('-', '').ToLowerInvariant() } finally { $sha.Dispose() }
    # Fixture local sintético, sin endpoints ocultos ni claves en texto plano.
    $seed = @"
BEGIN;
INSERT INTO partners(id,nombre,api_key_hash) VALUES ('$partner','Demo etapa 3',decode('$hash','hex'));
INSERT INTO cuentas(id,alumno_id) VALUES ('$cuenta','$alumno');
INSERT INTO vinculaciones(partner_id,external_user_id,cuenta_id) VALUES ('$partner','$usuario','$cuenta');
COMMIT;
"@
    $seed | docker compose --env-file .env -f deploy/docker-compose.yml exec -T postgres sh -c 'PGPASSWORD="$BENEFITS_DB_PASSWORD" exec psql -v ON_ERROR_STOP=1 -U benefits_user -d benefits_db' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Falló la carga del fixture sintético.' }

    function Send-Request([string]$Method, [string]$Path, [string]$Body, [string]$Key, [string]$Credential) {
        $request = [System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::new($Method), $BaseUrl + $Path)
        try {
            $request.Headers.Add('X-API-Key', $Credential)
            $request.Headers.Add('X-Partner-Id', 'identidad-falsa-del-cliente')
            $correlation = [guid]::NewGuid().ToString()
            $traceId = [guid]::NewGuid().ToString('N')
            $request.Headers.Add('X-Correlation-ID', $correlation)
            if ($Observability) { $request.Headers.Add('traceparent', ('00-' + $traceId + '-0123456789abcdef-01')) }
            if ($Key) { $request.Headers.Add('Idempotency-Key', $Key) }
            if ($Method -eq 'POST') { $request.Content = [System.Net.Http.StringContent]::new($Body, [Text.Encoding]::UTF8, 'application/json') }
            $response = $client.SendAsync($request).GetAwaiter().GetResult()
            try {
                $text = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
                $replayed = $response.Headers.Contains('Idempotent-Replayed') -and ($response.Headers.GetValues('Idempotent-Replayed') -contains 'true')
                return [pscustomobject]@{ TraceId = $traceId; CorrelationId = $correlation; Status = [int]$response.StatusCode; Body = $text; Data = ($text | ConvertFrom-Json); Replayed = $replayed }
            } finally { $response.Dispose() }
        } finally { $request.Dispose() }
    }
    $path = '/partner-api/v1/accounts/' + $usuario
    $body = '{"points":100,"reason":"Compra de prueba","reference":"demo-etapa-3"}'
    $key = [guid]::NewGuid().ToString()
    $credit = Send-Request 'POST' ($path + '/credits') $body $key $apiKey
    if ($credit.Status -ne 201) { throw 'La acreditación por gateway no devolvió 201.' }
    $replay = Send-Request 'POST' ($path + '/credits') $body $key $apiKey
    if ($replay.Status -ne 201 -or -not $replay.Replayed -or $replay.Body -cne $credit.Body) { throw 'El replay no conserva la respuesta original.' }
    $balance = Send-Request 'GET' ($path + '/balance') '' '' $apiKey
    if ($balance.Status -ne 200 -or $balance.Data.balance -ne 100) { throw 'Saldo incorrecto.' }
    $movements = Send-Request 'GET' ($path + '/movements?page=1&size=20') '' '' $apiKey
    if ($movements.Status -ne 200 -or $movements.Data.totalItems -ne 1) { throw 'Se esperaba exactamente un movimiento.' }
    $conflict = Send-Request 'POST' ($path + '/credits') ($body.Replace('100', '101')) $key $apiKey
    if ($conflict.Status -ne 409 -or $conflict.Data.code -ne 'IDEMPOTENCY_CONFLICT') { throw 'Conflicto contractual incorrecto.' }
    $unauthorized = Send-Request 'GET' ($path + '/balance') '' '' 'credencial-invalida'
    if ($unauthorized.Status -ne 401 -or $unauthorized.Data.code -ne 'UNAUTHORIZED') { throw 'Autenticación incorrecta.' }
    $missing = Send-Request 'GET' '/partner-api/v1/accounts/no-vinculado/balance' '' '' $apiKey
    if ($missing.Status -ne 404 -or $missing.Data.code -ne 'ACCOUNT_NOT_FOUND') { throw 'Aislamiento incorrecto.' }
    Write-Output 'E2E Gateway -> Benefits -> PostgreSQL: OK (201, replay idéntico, saldo 100, 1 movimiento, 409, 401, 404).'
    if ($Observability) {
        # Espera acotada solo para la exportación asíncrona de la evidencia.
        $deadline = [DateTime]::UtcNow.AddSeconds(60)
        $trace = $null
        do {
            try { $trace = Invoke-RestMethod ($JaegerUrl + '/api/traces/' + $credit.TraceId) } catch { $trace = $null }
            if ($trace -and $trace.data -and $trace.data[0].spans.Count -ge 5) { break }
            Start-Sleep -Milliseconds 1000
        } while ([DateTime]::UtcNow -lt $deadline)
        if (-not $trace -or -not $trace.data -or $trace.data[0].spans.Count -lt 5) { throw 'No se recibió la traza completa en Jaeger.' }
        $data = $trace.data[0]
        $services = @($data.processes.PSObject.Properties | ForEach-Object { $_.Value.serviceName })
        if ($services -notcontains 'api-gateway' -or $services -notcontains 'benefits-service') { throw 'Falta un servicio en la traza distribuida.' }
        if (@($data.spans | Where-Object { $_.operationName -eq 'postgres.transaccion' }).Count -ne 1) { throw 'Falta el span de persistencia de la acreditación.' }
        foreach ($span in $data.spans) {
            if ($span.traceID -ne $credit.TraceId) { throw 'La traza contiene identificadores inconsistentes.' }
        }
        $rawLogs = docker compose --env-file .env -f deploy/docker-compose.yml logs --no-color --no-log-prefix --since 5m api-gateway benefits-service
        if ($LASTEXITCODE -ne 0) { throw 'No se pudieron recuperar los logs.' }
        $logs = @($rawLogs | ForEach-Object {
            try { $entry = $_ | ConvertFrom-Json; if ($entry.trace_id -eq $credit.TraceId) { $entry } } catch { }
        })
        if (@($logs | Where-Object { $_.service -eq 'api-gateway' }).Count -lt 1 -or @($logs | Where-Object { $_.service -eq 'benefits-service' }).Count -lt 2) { throw 'Faltan logs correlacionados del flujo.' }
        foreach ($entry in $logs) {
            if ($entry.correlation_id -ne $credit.CorrelationId -or -not $entry.span_id) { throw 'El contexto de logs no coincide con la petición.' }
        }
        $traceJson = $trace | ConvertTo-Json -Depth 100
        $logsJson = ($logs | ForEach-Object { $_ | ConvertTo-Json -Compress -Depth 20 }) -join "`n"
        foreach ($sensitive in @($apiKey, $hash, $usuario, $partner, $cuenta, $alumno, $key, 'Compra de prueba', 'demo-etapa-3')) {
            if ($traceJson.Contains($sensitive) -or $logsJson.Contains($sensitive)) { throw 'La evidencia contiene datos que deben excluirse.' }
        }
        New-Item -ItemType Directory -Force -Path $EvidenceDirectory | Out-Null
        [IO.File]::WriteAllText((Join-Path (Resolve-Path $EvidenceDirectory) 'trace.json'), $traceJson, [Text.UTF8Encoding]::new($false))
        [IO.File]::WriteAllText((Join-Path (Resolve-Path $EvidenceDirectory) 'logs.jsonl'), $logsJson + "`n", [Text.UTF8Encoding]::new($false))
        Write-Output ('Observabilidad: OK; trace_id=' + $credit.TraceId + '; evidencia en ' + $EvidenceDirectory)
    }
    Write-Output 'El fixture sintético se conserva para respetar el ledger. La API key temporal no se imprime ni se guarda.'
} finally {
    $client.Dispose()
    Pop-Location
}
