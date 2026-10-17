# Etapa 4 — Logs correlacionados y primera traza distribuida

Fecha: 16/10/2026. Rama: `feature/entrega-2-etapa-04-observabilidad`, creada desde `origin/main` después del merge del PR #2. Estado: completada e integrada en `main` mediante el PR #3 (`8b0da47`). Código: `c5da897`.

## Alcance y requisitos

Se instrumenta el flujo mínimo de Entrega 2: Partner → Gateway → Benefits → PostgreSQL. Se conserva HU-29, HU-32, CU-06, RN-22, RN-26, RN-27 y CL-08/08b/08c; no se cambian reglas de negocio ni el contrato público. D11 se demuestra mediante logs JSON y una traza distribuida. D-48 aprueba el diseño inicial; D-55 aprueba historial de Jaeger en memoria y evidencia exportada. Ver [ADR-011 v2](adr/ADR-011-v2-observabilidad-local.md).

Fundamento: correlación/trazabilidad, pp. 7–8 del teórico; señales de resiliencia, pp. 111–112; independencia de dominio/adaptadores, pp. 120–124. OpenTelemetry, Collector y Jaeger son herramientas adicionales aprobadas, necesarias para obtener y visualizar la primera traza exigida; sus productos y configuración no se atribuyen al PDF. Solo logs no muestran relaciones y duración de spans. No se agrega otra base de observabilidad.

## Implementación

- `pkg/telemetry`: SDK Go 1.34, exportación OTLP/HTTP asíncrona, muestreo local del 100 %, middleware Gin y transporte HTTP con W3C traceparent. Configuración en los ejecutables de Gateway y Benefits. Sin endpoint configurado no se exportan trazas.
- Instrumentación manual de HTTP con atributos permitidos: método normalizado, ruta plantilla y estado. Se evita la captura automática de URL, query, headers o cuerpos. No se propaga baggage ni metadata tracestate del cliente.
- La recuperación de pánicos conserva el evento genérico y excluye su valor crudo, que podría contener secretos. El test comprueba también los logs.
- `pkg/logger`: agrega trace_id/span_id del contexto activo junto con correlation_id y service. Las líneas de arranque sin petición no tienen contexto de traza.
- Adaptador HTTP de Benefits: log del hecho de acreditación, con indicador de replay; errores operativos con estado y código, sin error crudo. No se exportan valores de puntos, motivo, referencia ni identificadores.
- Adaptador PostgreSQL: spans manuales para autenticación, transacción y listado; incluyen adquisición de conexión y trabajo del repositorio. No se capturan sentencias SQL, parámetros ni DSN. No se afirma instrumentación por sentencia.
- Collector recibe OTLP/HTTP y envía OTLP/gRPC a Jaeger por red de Compose. Solo Jaeger UI se publica en `127.0.0.1:16686`. Jaeger usa memoria; el historial se pierde al reiniciarlo. Batch y colas usan valores del fabricante; el límite de trazas reproduce su ejemplo oficial, sin presentarlo como capacidad medida.

La exportación es de mejor esfuerzo y no participa de la transacción de puntos. Collector no se agrega a la readiness crítica. El cierre intenta vaciar trazas dentro del SHUTDOWN_TIMEOUT existente. Una falla de exportación genera un aviso sanitizado; no se garantiza conservar trazas ante caída o saturación.

## Reproducción

Desde PowerShell, con Docker iniciado y `.env` local ya preparado:

```powershell
docker compose --env-file .env -f deploy/docker-compose.yml up -d jaeger otel-collector
docker compose --env-file .env -f deploy/docker-compose.yml up -d --build --no-deps --wait benefits-service api-gateway
./tests/e2e/verify-benefits.ps1 -Observability
```

El script crea datos sintéticos y una API key aleatoria, almacena solo su hash y no imprime la clave. Ejecuta acreditación/replay/consultas/errores por Gateway. Para la evidencia espera como máximo 60 s al pipeline asíncrono (límite del script, no del negocio), recupera la primera traza de Jaeger y los logs con su trace_id, y comprueba servicio, persistencia y correlación. Rechaza la exportación si encuentra credenciales, identificadores del fixture, clave idempotente o texto del cuerpo.

Los archivos se guardan en `docs/evidence/etapa4`; cada ejecución reemplaza la evidencia de esa carpeta. Puede indicarse otro destino con `-EvidenceDirectory`. Los fixtures se conservan para respetar la inmutabilidad del ledger.

Abrir [Jaeger local](http://localhost:16686), elegir `api-gateway` y buscar la operación de créditos. También puede consultarse la traza por su ID o importar `trace.json` desde la interfaz. La evidencia exportada permanece disponible aunque se reinicie Jaeger.

## Evidencia real

- [Traza exportada](evidence/etapa4/trace.json), `177a2bc2d4774b20899d288eb5e53267`.
- [Tres líneas de logs JSON](evidence/etapa4/logs.jsonl), con correlation_id `f6950893-9e52-47ed-a78a-804b0716ab4f`.
- Cinco spans: servidor Gateway → cliente HTTP Gateway → servidor Benefits → autenticación PostgreSQL y transacción PostgreSQL. Los dos spans de persistencia son hijos del servidor Benefits.
- El span Gateway conserva un padre remoto sintético del script, utilizado para comprobar que el traceparent externo se continúa.
- Acreditación 201; replay 201 con cuerpo idéntico; saldo 100; un movimiento; conflicto 409; credencial inválida 401; cuenta no vinculada 404.
- Gateway/Benefits construidos con Go 1.22 en Docker; Collector y Jaeger reciben la traza real.
- Validación final de los siete módulos: `go test ./...`, `go vet ./...` y golangci-lint v2.1.6 correctos. Benefits incluyó PostgreSQL real (36,794 s en persistencia); también pasaron integraciones de los demás servicios. El ajuste final de privacidad de pánicos pasó sus tests y vet.
- Trece contenedores healthy y Collector/Jaeger en ejecución; estos últimos no declaran healthcheck de Docker, por lo que no se presentan como healthy.
- Prueba automatizada en `pkg/telemetry`: relaciones padre-hijo, traceparent, logs y ausencia de datos sensibles. No sustituye la evidencia de PostgreSQL real.

Los tiempos de la traza corresponden a una sola ejecución local con otras comprobaciones en curso. No son percentiles, SLA ni resultados de carga de D13.

## Pendiente

Métricas exportadas, tableros, alertas activas y validación de sus umbrales; Prometheus, Loki y Grafana; trazas de los demás servicios y mensajería. La siguiente etapa del plan es D9, que continúa pendiente de asignación de proveedor; después corresponde resiliencia y caída controlada. No se declara completa la Entrega 2.
