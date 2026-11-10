# Etapa 6 — Booking → Benefits

**Estado:** verificación técnica local cerrada el 10/11/2026. El E2E aprobó sus
**32 escenarios**; evidencia en [attendance-e2e.json](../evidence/entrega-final/etapa6/attendance-e2e.json).
Revisión de otro integrante e integración pendientes. Etapas 4 y 5 verificadas previamente; esta etapa
conecta sus hechos con el ledger y el historial propio de la aplicación.

Historias y reglas: HU-19, HU-22, HU-26, HU-35; RN-20, RN-21, RN-22, RN-38 y RN-42.
Se implementan las reglas aprobadas en ADR-004 v2 y ADR-005 v2. La reducción de
alcance D-76 conserva la API de partners sin débitos/canjes: la penalización interna
por ausencia proviene de RN-38, no de una nueva operación pública para partners.

## 1. Flujo e invariantes

1. Booking guarda asistencia/ausencia y su outbox en la misma transacción (etapa 4).
2. El relay lee hasta 100 pendientes con `FOR UPDATE SKIP LOCKED`, publica eventos
   persistentes con confirms y marca `publicado_en` después de la confirmación.
   Declara ambas colas aunque Benefits esté apagado. Fallar el broker no revierte
   la reserva, el ingreso ni el cierre de clase.
3. Benefits consume `benefits.asistencia-registrada` y
   `benefits.inasistencia-registrada` con ACK manual. El adaptador valida tipo,
   versión, productor, IDs, fecha y el importe esperado. La regla local decide
   +500 o `-min(100,saldo)`; no toma importes arbitrarios del broker.
4. El recibo por `event_id`, el movimiento y la actualización de cuenta se confirman
   en una transacción de `benefits_db`. La fila de cuenta se bloquea con
   `SELECT FOR UPDATE`, igual que en los créditos HTTP existentes.
5. La secuencia de aplicación es canónica. La fecha original no reordena movimientos
   ya confirmados. Según el orden, desde cero una asistencia y una ausencia pueden
   dejar saldo 400 o 500, como establece RN-42.
6. Una ausencia con saldo cero genera un movimiento de 0 y consume su secuencia.
   No crea deuda ni vuelve a descontar después de recibir nuevos créditos.
7. Se deduplica por `event_id` y por `(tipo, origen_id)`: asistencia usa
   `asistencia_id`; ausencia usa `reserva_id`. Un nuevo event_id del mismo hecho
   agrega recibo pero no movimiento ni secuencia. Contenido incompatible con el
   ID/origen ya registrado se rechaza; UUID y fechas se normalizan para el replay.
8. Si el evento llega antes del alta asíncrona de cuenta, Benefits crea una cuenta
   vacía por el `alumno_id` confiable del hecho de Booking. La unicidad del alumno
   hace que el registro externo posterior adopte la misma cuenta y conserve puntos.

## 2. Persistencia y contratos

La migración `006_booking_events.sql` extiende el ledger existente. No cambia las
migraciones históricas ni usa tablas paralelas de saldo:

- `CREDITO_PARTNER`: partner obligatorio, origen interno nulo, importe 1 a 1.000.000.
- `ASISTENCIA`: partner nulo, origen obligatorio, importe +500.
- `PENALIZACION_INASISTENCIA`: partner nulo, origen obligatorio, importe −100 a 0.

Se conservan inmutabilidad, saldo no negativo, unicidad y secuencia. La base exige
recibo compatible para movimientos Booking al confirmar; los recibos son
inmutables. Un índice único protege el hecho de origen además del control de Go.
La migración también se prueba desde un ledger previo con historia.

Los créditos y movimientos de la API pública de partners conservan su contrato;
cada partner sigue viendo solo sus propios movimientos, aunque el saldo de cuenta
incluya otros orígenes. El historial autenticado del usuario ve toda su cuenta.

Esquemas de eventos:

- [asistencia.registrada.v1.json](../events/asistencia.registrada.v1.json).
- [inasistencia.registrada.v1.json](../events/inasistencia.registrada.v1.json).

Los campos adicionales se toleran. `traceparent` se publica vacío porque el outbox
no conserva aún la traza; completar la propagación pertenece a la etapa 14.

## 3. Aplicación web y autorización

`GET /api/v1/benefits/me/movements` implementa el contrato de
[benefits.v1](../api/benefits.v1.md). Solo acepta `pagina` y `tamano`; la identidad
proviene del JWT validado en el gateway. No permite elegir otro usuario por query.
Benefits requiere también la clave interna, devuelve RFC 7807 y no guarda caché.

El historial aparece en `#/app` y `#/cuenta`: saldo, movimientos con signo, ausencia
de 0 puntos, estado vacío/error/cuenta pendiente, paginación y actualización manual.
La secuencia ordena el historial; el saldo puede tardar unos instantes en converger.
El frontend usa el cliente HTTP y la sesión existentes, sin acceder al broker.

## 4. Activación local

Además de la configuración de identidad y registro de las etapas anteriores:

```text
BOOKING_ENABLED=true
BOOKING_MESSAGING_ENABLED=true
BENEFITS_MESSAGING_ENABLED=true
BENEFITS_EXTERNAL_REGISTRATION_ENABLED=true
```

Compose forma las URLs AMQP con las credenciales locales existentes. Para binarios
fuera de Compose configurar `BOOKING_AMQP_URL` y `BENEFITS_AMQP_URL`. Ambos flags de
mensajería están apagados por defecto; esta etapa no publica cambios en Vercel.

RabbitMQ es dependencia parcial en readiness de Booking y Benefits: la HTTP sigue
operativa con PostgreSQL disponible y los eventos esperan/reintentan. Cada cola
usa el esquema de reintentos 10 s, 1 min y 10 min, hasta cuatro errores, y DLQ para
errores permanentes o agotados. Los reintentos son quorum y la transferencia TTL
usa la garantía `at-least-once` verificada en etapa 5. El ACK ocurre después del
commit del efecto o de la confirmación del traslado a reintento/DLQ.

En Benefits, `/metrics` expone `booking_events_processed_total`,
`booking_events_retries_total` y `booking_events_dlq_total`. Son contadores de
instancia, no recibos durables; estos últimos están en PostgreSQL. La UI de RabbitMQ
permite observar las dos colas, reintentos y DLQ; el backlog productor se consulta
en `booking_db.outbox WHERE publicado_en IS NULL`.

## 5. Verificación

Desde la raíz con Docker operativo:

```text
go run ./tests/e2e/identity/main.go -attendance
```

Incluye la regresión de etapas 4/5 y agrega saldo e historial propios, permisos,
paginación, replay del outbox, nuevo event_id con mismo origen, caída de Benefits
durante reserva/ingreso/cierre, recuperación, replay después de reiniciar el
consumidor y mensaje con importe inválido hacia DLQ. Usa solo contenedores y bases
aisladas. Evidencia:
`docs/evidence/entrega-final/etapa6/attendance-e2e.json`.

`TestLedger_PostgreSQLReal/Booking` cubre saldos 0/50/500, normalización de replay,
inmutabilidad, diez reentregas concurrentes, crédito HTTP concurrente, rollback,
restricciones de base y separación del historial del partner. Los tests de dominio
verifican también ambos órdenes y overflow; el adaptador prueba importes inválidos
y versiones desconocidas. Gateway prueba la eliminación de identidad falsificada.

Tests/vet de Benefits, Booking, Gateway y `pkg`, build TypeScript/Vite y E2E de
32 escenarios aprobados. No hay automatización
visual del navegador; se mantiene para el cierre de la etapa 11.

## 6. Límites que continúan pendientes

- Los mensajes que agotan reintentos quedan en DLQ, no se consideran aplicados.
  Su reenvío operativo y los tableros/trazas completos pertenecen al cierre de
  resiliencia y observabilidad; no existe aquí una herramienta de replay masivo.
- No se incorpora el consumidor de bajas/cancelaciones de membresía en Booking,
  búsqueda OpenSearch, caché, nuevas operaciones de partners ni integración MELT.
- Revisión de otro integrante e integración por PR pendientes; no se hicieron
  commits, push ni despliegues.
