# Comprueba acreditacion, saldo, movimientos e idempotencia sin imprimir la API key.
[CmdletBinding()]
param(
    [string]$BaseUrl = 'https://metalfitness-club.vercel.app'
)

$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$secureKey = Read-Host 'Pega la API key NUEVA del partner de prueba' -AsSecureString
$buffer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secureKey)
try {
    $apiKey = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($buffer)
} finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($buffer)
}

try {
    if ($apiKey -notmatch '^[0-9a-fA-F]{64}$') {
        throw 'La API key debe tener 64 caracteres hexadecimales.'
    }
    $externalUserId = Read-Host 'Pega el externalUserId que mostro la prueba de registro'
    if ($externalUserId -notmatch '^[a-zA-Z0-9._:@-]{1,64}$') {
        throw 'El externalUserId no tiene un formato valido. Copia solo el valor, sin externalUserId=.'
    }

    $accountUrl = "$($BaseUrl.TrimEnd('/'))/partner-api/v2/accounts/$externalUserId"
    $queryHeaders = @{ 'X-API-Key' = $apiKey }
    $before = Invoke-RestMethod -Method Get -Uri "$accountUrl/balance" -Headers $queryHeaders
    $id = [Guid]::NewGuid().ToString('N')
    $eventId = "prueba-credito-$id"
    $headers = @{ 'X-API-Key' = $apiKey; 'Idempotency-Key' = "credito-$id" }
    $body = @{ eventId = $eventId; points = 25; reason = 'Prueba de integracion'; reference = $eventId } | ConvertTo-Json -Compress

    $credit = Invoke-RestMethod -Method Post -Uri "$accountUrl/credits" -Headers $headers -ContentType 'application/json' -Body $body
    $after = Invoke-RestMethod -Method Get -Uri "$accountUrl/balance" -Headers $queryHeaders
    if ([int]$after.balance -ne ([int]$before.balance + 25)) {
        throw "El saldo no subio 25 puntos: antes=$($before.balance), despues=$($after.balance)."
    }

    $replayed = Invoke-RestMethod -Method Post -Uri "$accountUrl/credits" -Headers $headers -ContentType 'application/json' -Body $body
    $afterReplay = Invoke-RestMethod -Method Get -Uri "$accountUrl/balance" -Headers $queryHeaders
    if ([int]$afterReplay.balance -ne [int]$after.balance) {
        throw 'Repetir la solicitud acredito puntos dos veces.'
    }
    if ($replayed.movementId -ne $credit.movementId) {
        throw 'La repeticion no devolvio el mismo movimiento.'
    }

    $movements = Invoke-RestMethod -Method Get -Uri "$accountUrl/movements?page=1&size=20" -Headers $queryHeaders
    if (-not @($movements.items | Where-Object { $_.reference -eq $eventId }).Count) {
        throw 'La acreditacion no aparece en los movimientos del partner.'
    }

    Write-Output "Acreditacion correcta: saldo $($before.balance) -> $($after.balance) puntos."
    Write-Output 'Idempotencia correcta: repetir el pedido no sumo otros 25 puntos.'
    Write-Output 'Movimientos correctos: la acreditacion figura en el historial del partner.'
    Write-Output 'Ahora toca Actualizar saldo en la pagina del Club: deberia mostrar el nuevo saldo.'
} catch {
    $response = $_.Exception.Response
    if ($null -ne $response) {
        $reader = New-Object System.IO.StreamReader($response.GetResponseStream())
        try { $responseBody = $reader.ReadToEnd() } finally { $reader.Dispose() }
        Write-Error "La API respondio HTTP $([int]$response.StatusCode): $responseBody"
    } else {
        Write-Error $_.Exception.Message
    }
    exit 1
} finally {
    Remove-Variable apiKey -ErrorAction SilentlyContinue
}
