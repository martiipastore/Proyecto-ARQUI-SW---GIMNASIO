<#
Linea base de la entrega final (etapa 0, docs/entrega-final/ETAPA_0_LINEA_BASE.md).

Ejecuta, en orden y sin detenerse ante una falla, las verificaciones reproducibles
del repositorio y deja el resultado en docs/evidence/entrega-final/etapa0/linea-base.json.
No modifica codigo, no usa .env para el E2E y no borra volumenes ni contenedores.

Requisitos: Go 1.22+, Docker Desktop iniciado (integraciones, Spectral y E2E) y
Node 20 con npm (frontend). Uso, desde la raiz del repositorio:

  powershell -NoProfile -ExecutionPolicy Bypass -File scripts/linea-base.ps1
  powershell -NoProfile -ExecutionPolicy Bypass -File scripts/linea-base.ps1 -SinRyuk -OmitirE2E

-SinRyuk           desactiva el contenedor de limpieza de Testcontainers (util si falla al arrancar Docker).
-OmitirE2E         no ejecuta el E2E de identidad y membresias (tarda varios minutos).
-OmitirProduccion  no consulta el despliegue publico.
#>
param(
    [switch]$SinRyuk,
    [switch]$OmitirE2E,
    [switch]$OmitirProduccion,
    [string]$UrlProduccion = 'https://metalfitness-club.vercel.app'
)

$ErrorActionPreference = 'Continue'
$root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
Set-Location $root

$results = [Collections.Generic.List[object]]::new()
function Registrar([string]$Nombre, [string]$Estado, [double]$Segundos, [string]$Detalle = '') {
    $results.Add([pscustomobject]@{ chequeo = $Nombre; estado = $Estado; segundos = [math]::Round($Segundos, 1); detalle = $Detalle })
    $color = 'Green'
    if ($Estado -eq 'FALLA') { $color = 'Red' } elseif ($Estado -eq 'OMITIDO') { $color = 'Yellow' }
    Write-Host ('[{0}] {1} {2}' -f $Estado, $Nombre, $Detalle) -ForegroundColor $color
}
# Ejecuta un comando nativo en una carpeta y registra su resultado por codigo de salida.
function Paso([string]$Nombre, [string]$Carpeta, [scriptblock]$Comando) {
    $inicio = Get-Date
    Push-Location (Join-Path $root $Carpeta)
    try {
        & $Comando
        $codigo = $LASTEXITCODE
    } catch {
        $codigo = 1
    } finally {
        Pop-Location
    }
    $estado = 'OK'
    if ($codigo -ne 0) { $estado = 'FALLA' }
    Registrar $Nombre $estado ((Get-Date) - $inicio).TotalSeconds
}
function Disponible([string]$Comando) { return [bool](Get-Command $Comando -ErrorAction SilentlyContinue) }

# `docker info` puede devolver 0 aunque el daemon no arranque; `docker version`
# falla en ese caso y, si responde, devuelve un numero de version.
$dockerOk = $false
$dockerVersion = ''
if (Disponible 'docker') {
    $dockerVersion = (& docker version --format '{{.Server.Version}}' 2>$null) -join ''
    $dockerOk = ($LASTEXITCODE -eq 0 -and $dockerVersion -match '^\d+\.\d+')
}
$entorno = [ordered]@{
    fecha_utc = [DateTime]::UtcNow.ToString('o')
    commit    = (& git rev-parse --short HEAD)
    rama      = (& git rev-parse --abbrev-ref HEAD)
    go        = $(if (Disponible 'go') { (& go version) } else { 'no instalado' })
    docker    = $(if ($dockerOk) { $dockerVersion } else { 'no disponible' })
    node      = $(if (Disponible 'node') { (& node --version) } else { 'no instalado' })
}

# 1. Go: vet y tests de cada modulo. Con Docker, las integraciones son obligatorias.
$env:REQUIRE_DOCKER_TESTS = $(if ($dockerOk) { '1' } else { '' })
if ($SinRyuk) { $env:TESTCONTAINERS_RYUK_DISABLED = 'true' }
$modulos = @('pkg', 'gateway', 'services/members-service', 'services/booking-service', 'services/benefits-service', 'services/training-service', 'services/notification-worker')
foreach ($m in $modulos) {
    Paso "go vet $m" $m { go vet ./... }
    Paso "go test $m" $m { go test -count=1 -timeout 15m ./... }
}
if (-not $dockerOk) {
    Registrar 'integraciones con testcontainers' 'OMITIDO' 0 'Docker no disponible: los tests de integracion se saltearon'
}
Paso 'inyector de resiliencia (tests y vet)' 'gateway' {
    go test -count=1 ../tests/resilience/fault-proxy/main.go ../tests/resilience/fault-proxy/main_test.go
    if ($LASTEXITCODE -eq 0) { go vet ../tests/resilience/fault-proxy/main.go ../tests/resilience/fault-proxy/main_test.go }
}
Paso 'diagnostico de pool (vet)' 'services/benefits-service' { go vet ../../tests/load/pool-info.go }
Paso 'ejecutor E2E (vet)' '.' { go vet ./tests/e2e/identity/main.go }

# 2. Frontend: instalacion reproducible, tipos y build.
if (Disponible 'npm') {
    Paso 'frontend npm ci' 'web' { npm ci --no-audit --no-fund }
    Paso 'frontend tsc + build' 'web' { npm run build }
} else {
    Registrar 'frontend' 'OMITIDO' 0 'npm no instalado'
}

# 3. Contratos y Compose.
if ($dockerOk) {
    foreach ($contrato in @('docs/contracts/benefits-api.v2.yaml', 'docs/contracts/benefits-api.v1.yaml')) {
        Paso "Spectral $contrato" '.' { docker run --rm -v "${root}:/work" -w /work stoplight/spectral:6.14.3 lint $contrato --ruleset docs/contracts/.spectral.yaml --fail-severity=warn }
    }
    $envFile = '.env.example'
    if (Test-Path '.env') { $envFile = '.env' }
    Paso 'docker compose config' '.' { docker compose --env-file $envFile -f deploy/docker-compose.yml config --quiet }
} else {
    Registrar 'Spectral y Compose' 'OMITIDO' 0 'Docker no disponible'
}

# 4. E2E aislado: servicios reales, PostgreSQL y Mailpit propios; no usa .env.
if ($OmitirE2E) {
    Registrar 'E2E identidad y membresias' 'OMITIDO' 0 'omitido por parametro'
} elseif ($dockerOk) {
    Paso 'E2E identidad y membresias (14 escenarios)' '.' { go run ./tests/e2e/identity/main.go -memberships }
} else {
    Registrar 'E2E identidad y membresias' 'OMITIDO' 0 'Docker no disponible'
}

# 5. Despliegue publico: solo lecturas sin credenciales; no crea datos.
if (-not $OmitirProduccion) {
    $esperados = [ordered]@{
        '/'                                           = 200
        '/partner-api/v2/accounts/linea-base/balance' = 401
        '/api/v1/users/me'                            = 401
        '/api/v1/internal/linea-base'                 = 404
    }
    foreach ($ruta in $esperados.Keys) {
        $inicio = Get-Date
        try {
            $respuesta = Invoke-WebRequest -UseBasicParsing -Uri ($UrlProduccion + $ruta) -TimeoutSec 20
            $codigo = [int]$respuesta.StatusCode
        } catch {
            $codigo = 0
            if ($_.Exception.Response) { $codigo = [int]$_.Exception.Response.StatusCode }
        }
        $estado = 'FALLA'
        if ($codigo -eq $esperados[$ruta]) { $estado = 'OK' }
        Registrar "produccion GET $ruta" $estado ((Get-Date) - $inicio).TotalSeconds ("esperado {0}, obtenido {1}" -f $esperados[$ruta], $codigo)
    }
}

# Evidencia.
$destino = Join-Path $root 'docs/evidence/entrega-final/etapa0'
New-Item -ItemType Directory -Force -Path $destino | Out-Null
$fallas = @($results | Where-Object { $_.estado -eq 'FALLA' }).Count
$omitidos = @($results | Where-Object { $_.estado -eq 'OMITIDO' }).Count
$informe = [ordered]@{ entorno = $entorno; resumen = [ordered]@{ total = $results.Count; fallas = $fallas; omitidos = $omitidos }; chequeos = $results.ToArray() }
[IO.File]::WriteAllText((Join-Path $destino 'linea-base.json'), ($informe | ConvertTo-Json -Depth 6), [Text.UTF8Encoding]::new($false))
Write-Host ('Linea base: {0} chequeos, {1} fallas, {2} omitidos. Evidencia: docs/evidence/entrega-final/etapa0/linea-base.json' -f $results.Count, $fallas, $omitidos)
if ($fallas -gt 0) { exit 1 }
