param([string]$BaseUrl = 'http://localhost:8080')
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
            $request.Headers.Add('X-Correlation-ID', [guid]::NewGuid().ToString())
            if ($Key) { $request.Headers.Add('Idempotency-Key', $Key) }
            if ($Method -eq 'POST') { $request.Content = [System.Net.Http.StringContent]::new($Body, [Text.Encoding]::UTF8, 'application/json') }
            $response = $client.SendAsync($request).GetAwaiter().GetResult()
            try {
                $text = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
                $replayed = $response.Headers.Contains('Idempotent-Replayed') -and ($response.Headers.GetValues('Idempotent-Replayed') -contains 'true')
                return [pscustomobject]@{ Status = [int]$response.StatusCode; Body = $text; Data = ($text | ConvertFrom-Json); Replayed = $replayed }
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
    Write-Output 'El fixture sintético se conserva para respetar el ledger. La API key temporal no se imprime ni se guarda.'
} finally {
    $client.Dispose()
    Pop-Location
}
