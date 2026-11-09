# Prueba el registro externo sin mostrar ni guardar la API key del partner.
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
        throw 'La API key debe tener 64 caracteres hexadecimales. Copia solo la clave nueva, sin el texto API key:.'
    }
    $email = Read-Host 'Correo REAL al que debe llegar la activacion'
    if ($email -notmatch '^[^@\s]+@[^@\s]+\.[^@\s]+$') {
        throw 'El correo de prueba no parece valido.'
    }
    $base = $BaseUrl.TrimEnd('/')
    $id = [Guid]::NewGuid().ToString('N')
    $eventId = "test-$id"
    $externalUserId = "test-$id"
    $headers = @{ 'X-API-Key' = $apiKey; 'Idempotency-Key' = "registro-$id" }
    $body = @{
        name = 'Persona de prueba'
        email = $email
        externalUserId = $externalUserId
        eventId = $eventId
        points = 0
    } | ConvertTo-Json -Compress

    $result = Invoke-RestMethod -Method Post -Uri "$base/partner-api/v2/external-register" -Headers $headers -ContentType 'application/json' -Body $body
    Write-Output "Registro aceptado: status=$($result.status), eventId=$eventId, externalUserId=$externalUserId"
    $queryHeaders = @{ 'X-API-Key' = $apiKey }
    for ($attempt = 1; $attempt -le 18; $attempt++) {
        Start-Sleep -Seconds 5
        $receipt = Invoke-RestMethod -Method Get -Uri "$base/partner-api/v2/external-registrations/$eventId" -Headers $queryHeaders
        Write-Output "Consulta $attempt`: status=$($receipt.status)"
        if ($receipt.status -eq 'completed') {
            Write-Output 'Registro completado. Revisa el correo de activacion, incluida la carpeta spam.'
            break
        }
        if ($receipt.status -eq 'failed') {
            throw 'El registro fallo. Revisa los logs de Benefits y del worker.'
        }
    }
    if ($receipt.status -ne 'completed') {
        throw 'El registro no se completo en 90 segundos. Revisa el worker y los logs.'
    }
} catch {
    $response = $_.Exception.Response
    if ($null -ne $response) {
        $reader = New-Object System.IO.StreamReader($response.GetResponseStream())
        try {
            $responseBody = $reader.ReadToEnd()
        } finally {
            $reader.Dispose()
        }
        Write-Error "La API respondio HTTP $([int]$response.StatusCode): $responseBody"
    } else {
        Write-Error $_.Exception.Message
    }
    exit 1
} finally {
    Remove-Variable apiKey -ErrorAction SilentlyContinue
}
