# Guion de demostración — Entrega 2

Fecha: 16/10/2026. Alcance: dominio propio y evidencia local. La revisión y sus pendientes están en [etapa 8](ETAPA_8_REVISION_ENTREGA.md).

## Preparación y ensayo reproducible

Requiere Docker Desktop, PowerShell, `.env` configurado y el gimnasio original levantado y `ready`. No sobrescribir `.env`. Desde la raíz:

```powershell
docker compose --env-file .env -f deploy/docker-compose.yml up -d --build --wait
./tests/e2e/review-delivery.ps1
```

El segundo comando detiene temporalmente el proyecto `gym` conservando sus volúmenes, construye una copia con otro nombre y volúmenes nuevos, verifica migraciones y tablas vacías, ejecuta flujo/traza y caída, retira los contenedores de prueba y restaura los contenedores que estaban funcionando, incluido el mock. No ejecutarlo simultáneamente con otra revisión. Los volúmenes de la copia se conservan para inspección; no se borran datos originales.

La evidencia se exporta a `docs/evidence/etapa8/copia-aislada`; al repetir, elegir una carpeta nueva con `-EvidenceDirectory`. El archivo `review.json` registra el proyecto concreto, estado inicial vacío, secuencia del ensayo y restauración. Los comandos de los helpers también aceptan `-ComposeProject`, cuyo valor predeterminado sigue siendo `gym`.

## 1. Alcance y decisiones

Mostrar [README](../README.md), [arquitectura](ARCHITECTURE.md), [registro](DECISIONES.md) y [contrato 1.1.1](contracts/benefits-api.v1.yaml). Benefits protege saldo, movimientos e idempotencia mediante PostgreSQL. D-42 registra la aclaración de cátedra comunicada por el equipo: alcanza como servicio operativo y capacidad compartida.

La implementación real cubre créditos, saldo y movimientos. Débitos, canjes y catálogo siguen como contrato/mock, sin negocio completo. La página web actual informa estado de servicios; no es todavía la interfaz funcional final.

## 2. Crédito y replay

Mostrar la salida del ensayo: crédito confirmado, replay byte a byte, saldo y movimiento único; conflicto si cambia el contenido, y rechazos de credencial o vinculación. La clave es temporal y no se imprime. El body del replay conserva la respuesta original; el header de correlación identifica el intento actual.

Los scripts son `tests/e2e/verify-benefits.ps1` y `verify-resilience.ps1`, invocados por el ensayo con el proyecto aislado. No dirigirlos al gimnasio original para fingir una prueba desde cero.

## 3. Traza y logs

Durante la ejecución de la copia puede abrirse `http://localhost:16686` y buscar el `trace_id` informado. Mostrar Gateway → Benefits → PostgreSQL y logs JSON con el mismo identificador. La exportación del flujo queda en la subcarpeta `flujo`.

Al restaurar el gimnasio original, la pantalla de Jaeger vuelve a su propia instancia y no conserva las trazas en memoria de la copia. Para revisar el ensayo después, usar los JSON exportados. No confundir un historial de pantalla perdido con falta de evidencia.

## 4. Falla y recuperación

Mostrar la subcarpeta `caida`: 502 por servicio inaccesible, 503 por base sin responder y 504 por timeout. Explicar que un timeout no implica rollback: reintentar con la misma clave recupera un resultado ya confirmado sin duplicar puntos.

Esperado en el fixture de caída: saldo 400, cuatro movimientos y cuatro operaciones. Mostrar `results.json`, trazas/logs y [postmortem](POSTMORTEM.md). La demostración también comprueba la restauración final; si falla, no presentar el ensayo como exitoso.

## 5. Capacidad y costos

Mostrar [etapa 7](ETAPA_7_CAPACIDAD_COSTOS.md), sus tablas y [costos](COSTOS_ENTREGA_2.md). No repetir toda la matriz durante una exposición: tiene 28 minutos activos más preparación.

La prueba con pausa limita el generador. Sin pausa se observó contención sobre una cuenta: a 10 VU, p95 472,24 ms y p99 superior a 1 s. No es capacidad garantizada en ARM. São Paulo identifica servidores; la conversión USD/ARS tiene fecha y fuente. No se contrató nube ni se midió demanda real; los 200 movimientos diarios son un supuesto aprobado.

## 6. Pendientes explícitos

D9/etapa 5 espera grupo, capacidad y contrato del proveedor; no hay integración ni test contractual del proveedor real. D11 tiene primera traza, pero faltan métricas continuas, tablero y alerta operativa. JWT, eventos/outbox, caché, búsqueda, balanceo y frontend completo pertenecen al desarrollo posterior. Bonus D-40 anunciados: Playwright y ocupación mediante SSE, todavía pendientes.

No declarar cerrado todo el plan de entrega mientras D9/etapa 5 siga sin resolver.
