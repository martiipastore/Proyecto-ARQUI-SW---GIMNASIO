# Validación de cierre — Entrega 2

Fecha: 17/10/2026. Rama: `docs/entrega-2-etapa-08-cierre`. Base validada: commit `a2f63e7`, posterior a los PR #1 a #5. Esta validación no cierra D9, que continúa bloqueada hasta recibir el proveedor y su contrato.

## Objetivo

Comprobar desde un entorno local preparado desde el repositorio que el código compila, los tests pasan contra dependencias reales, el contrato es válido y el flujo Partner → Gateway → Benefits → PostgreSQL puede demostrarse con logs y trazas. Las evidencias oficiales ya versionadas no se reemplazaron: las nuevas ejecuciones escribieron en carpetas temporales.

## Entorno

| Componente | Versión o estado |
|---|---|
| Sistema anfitrión | Windows, PowerShell |
| Go del anfitrión | 1.26.1 |
| Go de las imágenes | 1.22 |
| Node del anfitrión | 26.2.0 |
| Node de la imagen Web | 20 Alpine |
| Docker Engine | 29.7.2 |
| Docker Compose | 5.4.0 |
| Contrato | OpenAPI 3.1, Benefits API 1.1.1 |

Se creó `.env` desde `.env.example` y se comprobó que Git lo ignora. También se instalaron las dependencias del frontend con `npm ci`; `node_modules` y `dist` permanecen ignorados.

El puerto 27017 estaba ocupado por `clasecache-2026-mongo-1`. Se detuvo temporalmente ese contenedor, tal como ya estaba previsto en la línea base del plan, para levantar MongoDB de METALFITNESS. No se borraron contenedores ni volúmenes ajenos.

## Resultados

| Verificación | Resultado |
|---|---|
| Tests Go de los siete módulos con `-count=1` | Correcto |
| Integraciones de PostgreSQL y MongoDB mediante Testcontainers | Correcto; Docker estuvo disponible y las pruebas no fueron omitidas |
| Tests del inyector de resiliencia | Correcto |
| `go vet` en siete módulos y utilidades | Correcto |
| TypeScript `tsc --noEmit` | Correcto |
| Build del frontend | Correcto |
| Build de las ocho imágenes propias | Correcto |
| Configuración de Docker Compose | Correcta |
| Spectral 6.14.3 con severidad mínima `warn` | Sin errores ni advertencias |
| Stack completo | 15 contenedores en ejecución; 13 con health check saludable y Collector/Jaeger en ejecución |
| Estado agregado del Gateway | `ready`; seis procesos internos y sus dependencias disponibles |
| Web `/healthz` | HTTP 200 |
| Jaeger UI | HTTP 200 |

Los comandos principales fueron:

```powershell
go test -count=1 ./...
go vet ./...
npm --prefix web run lint
npm --prefix web run build
docker compose --env-file .env -f deploy/docker-compose.yml config --quiet
docker run --rm -v "${PWD}:/work" -w /work stoplight/spectral:6.14.3 lint docs/contracts/benefits-api.v1.yaml --ruleset docs/contracts/.spectral.yaml --fail-severity=warn
docker compose --env-file .env -f deploy/docker-compose.yml up -d --build --wait
```

Los comandos Go se ejecutaron secuencialmente dentro de cada módulo, igual que en el Makefile, para evitar interferencias entre contenedores de integración.

## Flujo principal y observabilidad

`tests/e2e/verify-benefits.ps1 -Observability` terminó correctamente a través del prefijo público del Gateway. Verificó:

- acreditación con HTTP 201;
- replay con la misma clave y cuerpo idéntico;
- saldo 100 y un movimiento;
- conflicto de idempotencia 409;
- credencial inválida 401;
- cuenta no vinculada 404;
- traza Gateway → Benefits → PostgreSQL;
- correlación entre spans y logs.

La ejecución produjo el `trace_id` `5d6d4ce1b05b43dcbc870d88e7f984c0`. La API key temporal no se imprimió ni se guardó. La evidencia oficial de la etapa 4 permanece en `docs/evidence/etapa4`.

## Caída controlada

`tests/e2e/verify-resilience.ps1` se repitió con evidencia temporal y terminó correctamente. Comprobó:

- Benefits detenido: 502;
- PostgreSQL pausado: 503;
- timeout antes de alcanzar Benefits: 504;
- respuesta perdida después del commit: 504;
- recuperación con la misma `Idempotency-Key` sin duplicar puntos;
- estado final de 400 puntos, cuatro movimientos y cuatro operaciones;
- restauración de PostgreSQL, Benefits y Gateway;
- retiro del proxy temporal.

El postmortem y la evidencia oficial se conservan en `docs/POSTMORTEM.md` y `docs/evidence/etapa6`.

## Hallazgos y límites

- Vite se actualizó de 5.4.11 a 5.4.21, la última versión 5 compatible con el proyecto. TypeScript, build y contenedor Web continuaron correctos.
- `npm audit` aún informa una vulnerabilidad alta y una moderada asociadas al servidor de desarrollo de Vite/esbuild. La corrección automática exige Vite 8, un cambio mayor que requiere revisar Node y plugins. El artefacto desplegado se compila en Docker y se sirve con Nginx; no se publica el servidor de desarrollo de Vite.
- `golangci-lint` 2.1.6 no está instalado en el anfitrión y no se volvió a ejecutar localmente. La configuración de CI continúa ejecutándolo; esta validación sí ejecutó `go vet` completo.
- No se repitieron las 36 fases de carga de la etapa 7 porque no cambió el backend. Se conservan sus JSON, invariantes y análisis versionados.
- D9 y la etapa 5 continúan bloqueadas hasta recibir una capacidad externa real. No se creó un mock que simule falsamente esa integración.

## Estado de la revisión

El sistema cumple las verificaciones técnicas reproducibles previstas para la etapa 8, excepto las dependencias externas señaladas. Antes de cerrar la etapa faltan preparar el guion de demostración, actualizar el checklist final y revisar los enlaces desde README y ARCHITECTURE.
