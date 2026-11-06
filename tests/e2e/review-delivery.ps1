param([string]$EvidenceDirectory = 'docs/evidence/etapa8/copia-aislada')
# D-61 / HU-29 / HU-32 / RN-22 / RN-26 / RN-27. Solo gimnasio local.
$ErrorActionPreference = 'Stop'
$OutputEncoding = [Text.UTF8Encoding]::new($false)
Push-Location (Resolve-Path (Join-Path $PSScriptRoot '../..'))
$project = 'gym-review-' + [guid]::NewGuid().ToString('N').Substring(0,8)
$base = @('compose','--env-file','.env','-f','deploy/docker-compose.yml')
$normal = $base + @('-p','gym')
$isolated = $base + @('-p',$project,'-f','tests/e2e/docker-compose.review.yml')
$restore = $false
$passed = $false
$restored = $false
$fresh = $null
$evidence = $null
$originalNames = @()
$webAssets = 0
$timeline = [Collections.Generic.List[object]]::new()
function Mark([string]$Action) { $timeline.Add([pscustomobject]@{utc=[DateTime]::UtcNow.ToString('o');action=$Action}) }
function Compose($Prefix, $Arguments) {
    & docker @Prefix @Arguments | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Fallo de Compose en la revision aislada.' }
}
try {
    New-Item -ItemType Directory -Force -Path $EvidenceDirectory | Out-Null
    $evidence = (Resolve-Path $EvidenceDirectory).Path
    $containers = & docker ps -aq --filter "label=com.docker.compose.project=$project"
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo comprobar el proyecto temporal.' }
    $volumes = & docker volume ls -q --filter "label=com.docker.compose.project=$project"
    if ($LASTEXITCODE -ne 0 -or $containers -or $volumes) { throw 'El proyecto temporal debe ser nuevo y estar vacio.' }
    Mark 'proyecto_nuevo_sin_contenedores_ni_volumenes'
    $originalNames = @(& docker ps --filter 'label=com.docker.compose.project=gym' --format '{{.Names}}')
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo registrar el entorno original.' }
    if ($originalNames.Count -eq 0) { throw 'Primero iniciar el gimnasio original; no ejecutar revisiones simultaneas.' }
    $originalStatus = Invoke-RestMethod 'http://localhost:8080/api/v1/status'
    if ($originalStatus.status -ne 'ready') { throw 'El gimnasio original debe estar listo antes del ensayo.' }
    $restore = $true
    Compose $normal @('stop')
    Mark 'gimnasio_original_detenido_volumenes_conservados'
    Compose $isolated @('up','-d','--build','--wait')
    Mark 'copia_aislada_construida_y_arrancada'
    $page = Invoke-WebRequest -UseBasicParsing 'http://localhost:5173'
    $assets = [regex]::Matches($page.Content, '(?:src|href)="(/assets/[^"]+)"')
    if ($page.StatusCode -ne 200 -or $assets.Count -lt 1) { throw 'El frontend compilado no se sirve correctamente.' }
    foreach ($asset in $assets) {
        $resource = Invoke-WebRequest -UseBasicParsing ('http://localhost:5173' + $asset.Groups[1].Value)
        if ($resource.StatusCode -ne 200) { throw 'Falta un recurso compilado del frontend.' }
        $webAssets++
    }
    Mark 'frontend_y_recursos_compilados_servidos_por_nginx'
    $sql = "SELECT json_build_object('accounts',(SELECT count(*) FROM cuentas),'movements',(SELECT count(*) FROM movimientos_puntos),'operations',(SELECT count(*) FROM operaciones_partner));"
    $raw = $sql | & docker @isolated exec -T postgres sh -c 'PGPASSWORD="$BENEFITS_DB_PASSWORD" exec psql -Atq -v ON_ERROR_STOP=1 -U benefits_user -d benefits_db'
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo verificar la base nueva.' }
    $fresh = ($raw -join "`n") | ConvertFrom-Json
    if ($fresh.accounts -ne 0 -or $fresh.movements -ne 0 -or $fresh.operations -ne 0) { throw 'La copia no comenzo con Benefits vacio.' }
    Mark 'migraciones_aplicadas_y_tablas_de_negocio_vacias'
    & "$PSScriptRoot/verify-benefits.ps1" -ComposeProject $project -Observability -EvidenceDirectory (Join-Path $evidence 'flujo')
    Mark 'credito_replay_saldo_movimientos_y_traza_verificados'
    & "$PSScriptRoot/verify-resilience.ps1" -ComposeProject $project -EvidenceDirectory (Join-Path $evidence 'caida')
    Mark 'caida_y_recuperacion_idempotente_verificadas'
    $passed = $true
} finally {
    try {
        if ($restore) {
            # No usar -v: se conservan tambien los volumenes de prueba para inspeccion.
            Compose $isolated @('down','--remove-orphans')
            Mark 'contenedores_de_copia_retirados_volumenes_de_prueba_conservados'
        }
    } finally {
        try {
            if ($restore) {
                Compose $normal @('up','-d','--wait')
                # Restaurar todos los contenedores iniciales, incluido el mock.
                foreach ($name in $originalNames) {
                    $running = & docker inspect $name --format '{{.State.Running}}'
                    if ($LASTEXITCODE -ne 0) { throw 'Falta un contenedor del entorno original.' }
                    if ($running -ne 'true') {
                        & docker start $name | Out-Null
                        if ($LASTEXITCODE -ne 0) { throw 'No se pudo restaurar un contenedor original.' }
                    }
                }
                $deadline = [DateTime]::UtcNow.AddSeconds(60)
                do {
                    $allHealthy = $true
                    foreach ($name in $originalNames) {
                        $state = (& docker inspect $name --format '{{json .State}}') | ConvertFrom-Json
                        if ($LASTEXITCODE -ne 0) { throw 'No se pudo comprobar la restauracion.' }
                        if (-not $state.Running -or ($state.Health -and $state.Health.Status -ne 'healthy')) { $allHealthy = $false }
                    }
                    if ($allHealthy) { break }
                    Start-Sleep -Seconds 1
                } while ([DateTime]::UtcNow -lt $deadline)
                if (-not $allHealthy) { throw 'No todos los contenedores originales recuperaron su salud.' }
                $status = Invoke-RestMethod 'http://localhost:8080/api/v1/status'
                if ($status.status -ne 'ready') { throw 'El gimnasio original no quedo listo.' }
                $restored = $true
                Mark 'gimnasio_original_restaurado_y_ready'
            }
        } finally {
            if ($evidence) {
                $summary = [pscustomobject]@{project=$project;passed=$passed;original_restored=$restored;original_containers=$originalNames;frontend_assets_verified=$webAssets;fresh_state=$fresh;timeline=$timeline.ToArray();test_volumes_retained=$true}
                [IO.File]::WriteAllText((Join-Path $evidence 'review.json'),($summary|ConvertTo-Json -Depth 20),[Text.UTF8Encoding]::new($false))
            }
            Pop-Location
        }
    }
}
Write-Output 'Revision aislada: OK; base nueva, flujo y caida verificados; gimnasio original restaurado.'
