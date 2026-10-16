# Etapa 3 — API de partners y Gateway

## Flujo implementado

`Partner → /partner-api/v1 → Gateway → /v1 → Benefits → PostgreSQL`.

Se implementan los tres endpoints del flujo mínimo: créditos, saldo y movimientos paginados. Se mantienen el contrato y el mock de las demás operaciones; débitos, canjes y catálogo real siguen pendientes. El cliente no puede crear cuentas ni vinculaciones desde esta API.

| Operación | Respuesta principal | Reglas |
|---|---|---|
| POST /partner-api/v1/accounts/{externalUserId}/credits | 201, mismo status/body en replay, Idempotent-Replayed | HU-29; RN-22/26/27; CL-08/08b/08c |
| GET /partner-api/v1/accounts/{externalUserId}/balance | Saldo total y updatedAt | HU-32; RN-22/26; fecha de creación si está vacía (D-53) |
| GET /partner-api/v1/accounts/{externalUserId}/movements | Solo movimientos del partner, fecha descendente y secuencia como desempate | HU-32; RN-26; page 1–100000, size 1–100, por defecto 1/20 |

## Autenticación y entrada

Autenticación y operación comparten un presupuesto total de 4 s, dentro de los 5 s del gateway. Benefits consulta el hash SHA-256 de X-API-Key en PostgreSQL en cada solicitud; no usa caché. Clave desconocida o revocada produce 401 UNAUTHORIZED y partner inactivo 403 FORBIDDEN. El estado del partner se comprueba nuevamente en la transacción del caso de uso. El gateway elimina cabeceras de identidad e internas recibidas del cliente; Benefits deriva siempre el partner desde su credencial.

El adaptador valida JSON, campos obligatorios, campos desconocidos, valores null, cuerpos concatenados, Content-Type, path, claves y paginación. No deja reglas de saldo o autorización en el gateway. Los datos de entradas inválidas se identifican mediante huella; no se persiste el body recibido. Validaciones sin identidad o sin clave válida ocurren antes de poder registrar una operación idempotente.

## Respuesta original e idempotencia

Un puerto de salida representa el resultado mediante un adaptador HTTP. La aplicación no conoce HTTP ni el formato de los bytes; los confirma opacamente junto al resultado del dominio, saldo y movimiento. PostgreSQL conserva status y cuerpo exactos, incluida la correlación original de un rechazo. Al repetir, el body conserva esa correlación original y el header X-Correlation-ID identifica el intento actual. Se retienen también los rechazos ejecutados con identidad y clave válidas.

Fundamento: pp. 121–124 del teórico, especialmente la interfaz de salida cuya implementación transforma datos al formato del cliente (p. 124); idempotencia y transacción local, pp. 27–28 y 103–105. Es aplicación del patrón aprobado, sin dependencias nuevas.

La unicidad SQL continúa evitando duplicados. D-54/ADR-015 agrega intento de bloqueo transaccional por partner/clave para devolver 409 IDEMPOTENCY_IN_PROGRESS sin esperar una ejecución ajena. Después de terminar, la misma clave recupera el original; otros datos producen IDEMPOTENCY_CONFLICT. Una colisión del hash de lock puede causar un 409 temporal, nunca duplicar efectos.

## Errores, correlación y logs

Se reutiliza pkg/problem para RFC 7807, con excepción pública camelCase/códigos en inglés. Las APIs internas conservan su formato. El gateway devuelve 502 UNAVAILABLE con Retry-After si no alcanza Benefits y 504 TIMEOUT si vence su espera. Benefits traduce fallas de almacenamiento a 503 UNAVAILABLE con Retry-After; errores inesperados producen 500 INTERNAL sin detalles internos.

Los identificadores de correlación de partners se normalizan al UUID que exige OpenAPI. Los logs registran rutas plantilla o prefijos seguros, sin externalUserId, API keys, cuerpos ni errores de transporte que incluyan URLs sensibles. Todavía no se afirma una traza distribuida: eso corresponde a etapa 4.

## Migraciones y fixture

El arranque de Benefits aplica las migraciones versionadas antes de exponer la API; el comando migrate sigue disponible. La migración 001 aplicada no se modifica: 002 agrega creada_en, conforme a D-53. En cuentas anteriores sin ese dato, el valor inicial representa la fecha de migración; no se inventa una fecha histórica conocida.

Con el stack levantado y desde la raíz:

```powershell
./tests/e2e/verify-benefits.ps1
```

El script carga un fixture sintético por SQL con el usuario de benefits_db, genera una clave temporal aleatoria y guarda solo el hash. No imprime ni escribe la clave. Recorre el gateway y verifica 201, replay byte a byte, saldo, movimiento único, conflicto, credencial inválida y cuenta no vinculada. Conserva el fixture para respetar la inmutabilidad del ledger; cada ejecución usa identidades nuevas. No contiene endpoints ocultos ni datos personales.

## Validación contractual reproducible

Las pruebas de integración pueden exportar respuestas sintéticas a un archivo temporal. Un helper Go reutiliza yaml.v3, ya presente en el workspace, para agregarlas como ejemplos al OpenAPI temporal. Spectral valida contra los esquemas originales, sin un validador casero ni dependencias nuevas.

```powershell
$evidence = Join-Path $env:TEMP 'gym-stage3-responses.json'
$runtimeContract = Join-Path $env:TEMP 'gym-stage3-openapi.json'
$env:BENEFITS_CONTRACT_EVIDENCE = $evidence
go test -count=1 ./services/benefits-service/...
Remove-Item Env:BENEFITS_CONTRACT_EVIDENCE
go run ./tests/contracts/runtime-examples.go $evidence $runtimeContract
docker run --rm -v "${PWD}:/work" -v "${runtimeContract}:/runtime.openapi.json:ro" -w /work stoplight/spectral:6.14.3 lint docs/contracts/benefits-api.v1.yaml /runtime.openapi.json --ruleset docs/contracts/.spectral.yaml --fail-severity=warn
```

Los tests también cubren revocación, suspensión, JSON inválido y rechazos persistidos byte a byte, páginas incorrectas, headers de correlación, aislamiento, 409 en curso y ausencia de identificadores/credenciales en logs. Los tests del gateway comprueban ruta, query, eliminación de identidad falsificada y errores 502/504.

## Evidencia del 16/10/2026

- Pruebas de Benefits con PostgreSQL real, pkg y gateway y go vet: correctas en las corridas de implementación.
- 23 respuestas capturadas, validadas con Spectral contra OpenAPI: sin errores ni advertencias.
- Stack construido y healthy; script E2E por el gateway: correcto.
- Validación final del workspace: tests y go vet de los siete módulos correctos; Benefits incluyó PostgreSQL real y persistencia del cuerpo HTTP exacto (22,561 s en el paquete de persistencia). Script E2E final correcto y trece contenedores healthy. No se omitieron los tests de integración de Benefits.
- No se presenta autenticación JWT general, rate limiting ni los demás endpoints del contrato como implementados. No se declara la Entrega 2 completa: faltan observabilidad y las siguientes etapas del plan.
