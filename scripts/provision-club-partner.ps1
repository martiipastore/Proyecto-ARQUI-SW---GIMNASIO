# Crea un partner en benefits_db sin mostrar ni guardar la URL de Neon.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true, ParameterSetName = 'Create')]
    [ValidateNotNullOrEmpty()]
    [string]$Name,
    [Parameter(Mandatory = $true, ParameterSetName = 'Rotate')]
    [ValidateNotNullOrEmpty()]
    [string]$RotateId,
    [Parameter(Mandatory = $true, ParameterSetName = 'ListUsers')]
    [ValidateNotNullOrEmpty()]
    [string]$ListUsersId
)

$ErrorActionPreference = 'Stop'
$secureDsn = Read-Host 'Pega SOLO la URL de DATABASE_URL_UNPOOLED (empieza con postgresql:// o postgres://)' -AsSecureString
$buffer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secureDsn)
try {
    $env:BENEFITS_DB_DSN = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($buffer)
} finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($buffer)
}

try {
    if ($env:BENEFITS_DB_DSN -notmatch '^postgres(ql)?://') {
        throw 'La conexion debe empezar con postgresql:// o postgres://. En Neon, pulsa Show secret y copia solo el valor despues de =, sin DATABASE_URL_UNPOOLED=.'
    }
    $repoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
    $servicePath = Join-Path $repoRoot 'services\benefits-service'
    if ($PSCmdlet.ParameterSetName -eq 'Rotate') {
        & go -C $servicePath run ./cmd/partneradmin -rotate-id $RotateId
    } elseif ($PSCmdlet.ParameterSetName -eq 'ListUsers') {
        & go -C $servicePath run ./cmd/partneradmin -list-users $ListUsersId
    } else {
        & go -C $servicePath run ./cmd/partneradmin -name $Name
    }
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
} finally {
    Remove-Item Env:\BENEFITS_DB_DSN -ErrorAction SilentlyContinue
}
