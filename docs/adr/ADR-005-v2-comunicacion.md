# ADR-005 v2 — Comunicación y garantías entre servicios

| Campo | Valor |
|---|---|
| **Estado** | Aceptado |
| **Fecha** | 2026-10-08 |
| **Decisión del TP** | D5 |
| **Reemplaza** | [ADR-005](ADR-005-comunicacion.md) |
| **Autores** | Equipo del TPI |
| **Relacionado con** | SPEC S-10, RN-04, RN-05, RN-12, RN-20, RN-21, RN-27 y RN-41; [ADR-001](ADR-001-limites-de-servicios.md); [ADR-003](ADR-003-persistencia.md); [ARCHITECTURE §6–8](../ARCHITECTURE.md#6-comunicaciones-síncronas-y-asíncronas) |

## Motivo de la revisión

La versión inicial calificó como “consistencia fuerte” la consulta síncrona de membresía antes de reservar. Una lectura HTTP fresca permite decidir con datos actuales, pero no crea una transacción entre `members_db` y `booking_db`: una cancelación puede confirmarse entre la consulta y la inserción de la reserva.

También era necesario precisar dos efectos externos al alta: la creación eventual de la cuenta de beneficios y la entrega de emails mediante SMTP, cuyo envío no puede confirmarse atómicamente junto con el registro del worker.

Esta revisión mantiene el enfoque híbrido, el outbox y los consumidores idempotentes, y define garantías que pueden implementarse y probarse.

## Decisión general

Se usa comunicación:

- **síncrona** cuando la respuesta inmediata es necesaria para decidir una operación;
- **asíncrona** para propagar hechos y ejecutar efectos que admiten demora;
- **Transactional Outbox** para no perder hechos al guardar un cambio;
- consumidores idempotentes con ACK posterior al efecto durable.

No se afirma atomicidad entre servicios. Cada invariante fuerte se protege dentro del servicio dueño de los datos; los cambios relacionados entre servicios convergen mediante eventos y reconciliación.

## Interacciones síncronas

| Interacción | Timeout | Reintentos | Ante falla |
|---|---|---|---|
| Frontend → gateway → servicio | 3 s lecturas / 5 s escrituras | Ninguno en gateway; el cliente usa la misma `Idempotency-Key` cuando corresponda | `502`/`504` RFC 7807 |
| Partner → gateway → benefits | 5 s | A cargo del partner con la misma `Idempotency-Key` | RFC 7807 |
| booking → members: vigencia | 800 ms | 1, solo por ser GET idempotente | Falla cerrado con `503`; no se reserva |
| booking → members: datos para listados | 800 ms | 1 | Degrada a IDs sin nombre |
| training → members: asignación | 800 ms | 1 | Falla cerrado |
| benefits → members: vinculación | 800 ms | 1 | `503`; el administrador reintenta |

Todos los clientes propagan `traceparent` y `X-Correlation-Id`, respetan el presupuesto temporal de la solicitud y solo reintentan operaciones idempotentes.

## Validación de membresía al reservar

1. Booking consulta a members inmediatamente antes de iniciar la transacción local de reserva.
2. Members responde si el alumno está ACTIVO y tiene membresía vigente hoy y en la fecha de la clase. Incluye el identificador de la membresía que cubre la fecha de la clase y el instante de evaluación.
3. Booking guarda ese identificador con la reserva.
4. La transacción que crea la reserva toma un bloqueo transaccional por `membresia_id` (`pg_advisory_xact_lock`) y comprueba que no exista una revocación local conocida antes de insertar.
5. Al cancelar una membresía, members guarda la cancelación y `membresia.cancelada` en su outbox.
6. Booking consume el hecho de forma idempotente usando el mismo bloqueo por `membresia_id`, registra durablemente la revocación y cancela las reservas futuras que ya no estén cubiertas por otra membresía. Si el evento obtiene el bloqueo primero, la inserción posterior ve la revocación y se rechaza; si la reserva obtiene el bloqueo primero, el consumidor la encuentra y la cancela.

La consulta reduce la ventana de datos obsoletos y permite fallar cerrado cuando members no responde. El evento y el registro de revocaciones resuelven la carrera, pero durante la propagación puede existir una reserva transitoriamente confirmada. La garantía final es **convergencia a un estado válido**, no una transacción distribuida.

## Interacciones asíncronas

| Evento | Productor → Consumidor | Efecto |
|---|---|---|
| `alumno.creado` | members → benefits | Crear una única cuenta con saldo 0 |
| `membresia.activada` | members → notification-worker | Crear y enviar la notificación lógica de confirmación |
| `membresia.recordatorio_vencimiento` / `membresia.advertencia_vencimiento` | members → notification-worker | Crear y enviar el aviso correspondiente |
| `asistencia.registrada` | booking → benefits | Acreditar 500 puntos de forma idempotente |
| `inasistencia.registrada` | booking → benefits | Aplicar la penalización de forma idempotente |
| `clase.actualizada` | booking → booking-indexer | Actualizar el modelo de lectura |
| `membresia.cancelada` / `usuario.desactivado` | members → booking | Revocar habilitación y cancelar reservas afectadas |

El orden entre movimientos de puntos se define en una decisión separada; esta ADR solo exige entrega al menos una vez e idempotencia por efecto.

## Creación de la cuenta de beneficios

El alta del alumno y `alumno.creado` se guardan en la misma transacción de members. La respuesta al administrador no espera a benefits.

Benefits crea la cuenta al consumir el evento:

- `alumno_id` es único en cuentas;
- `event_id` se registra en la misma transacción que la cuenta;
- una reentrega no crea otra cuenta;
- si benefits o RabbitMQ están temporalmente caídos, el evento permanece pendiente o se reintenta;
- el fallo no revierte ni bloquea el alta del alumno.

Por lo tanto, “creada automáticamente al darlo de alta” significa creación eventual iniciada por el alta, no disponibilidad atómica al responder members.

## Notificaciones por email

El worker garantiza una única **notificación lógica** por `(membresia_id, tipo)` y deduplica reentregas por `event_id`. Registra estados e intentos en MongoDB.

SMTP es un efecto externo no transaccional. Si el servidor acepta el mensaje y el worker cae antes de guardar `ENVIADA`, al recuperarse no puede saber con certeza si debe reenviarlo. El sistema reintenta para favorecer la entrega y acepta que esa ventana pueda producir un email duplicado.

La garantía es:

- la operación de membresía nunca espera ni se revierte por el email;
- no se crean dos notificaciones lógicas para el mismo propósito;
- los intentos y el resultado conocido quedan registrados;
- pueden existir duplicados de entrega ante un resultado SMTP indeterminado.

## Outbox, reintentos e idempotencia

1. El cambio de negocio y la fila de outbox se guardan en la misma transacción local.
2. El relay publica con publisher confirms y marca la fila como publicada; una caída puede causar reentrega, no pérdida.
3. Cada consumidor registra `event_id` en la misma transacción que su efecto durable y realiza ACK después del commit.
4. Los errores transitorios pasan por reintentos con demora; los permanentes o agotados terminan en DLQ.
5. Las restricciones del dominio constituyen una segunda barrera, como `UNIQUE (alumno_id)` para cuentas o el origen único de un movimiento.

La garantía del broker es **al menos una vez**. La deduplicación produce un único efecto durable dentro de la base del consumidor; no convierte efectos externos como SMTP en exactamente una vez.

## Consecuencias

### Positivas

- Las garantías distinguen atomicidad local, validación fresca y convergencia eventual.
- Una caída de benefits o SMTP no bloquea altas, membresías ni asistencias.
- Las carreras entre reserva y cancelación no dejan reservas inválidas de forma permanente.
- Las pruebas pueden verificar resultados observables sin asumir transacciones distribuidas.

### Negativas

- Una reserva puede aparecer confirmada durante la propagación de una cancelación concurrente.
- La cuenta de beneficios puede no estar disponible inmediatamente después del alta.
- Un email puede duplicarse en la ventana indeterminada de SMTP.
- Booking debe almacenar la membresía de respaldo, las revocaciones consumidas y serializar ambas operaciones por membresía.

## Cómo se verificará

- **Reserva contra cancelación:** ejecutar ambas operaciones en órdenes controlados, incluida la entrega del evento antes y después del insert; el estado convergente nunca conserva una reserva inválida.
- **Members caído:** reservar responde `503` dentro del timeout y no modifica cupo ni reservas.
- **Cuenta eventual:** detener benefits, crear un alumno, recuperar benefits y comprobar una sola cuenta con saldo 0 aun con reentrega del evento.
- **Outbox:** forzar una caída después del commit y comprobar que el evento se publica al recuperarse.
- **Idempotencia:** reentregar un evento varias veces y observar un solo efecto durable.
- **Email:** comprobar una única notificación lógica y documentar mediante una prueba de fallo la ventana donde el envío externo puede duplicarse.
