# ADR-005 — Comunicación síncrona y asíncrona entre servicios (versión inicial)

| Campo | Valor |
|---|---|
| **Estado** | Reemplazado por [ADR-005 v2](ADR-005-v2-comunicacion.md) |
| **Fecha** | 2026-10-07 |
| **Decisión del TP** | D5 |
| **Autores** | Equipo del TPI |
| **Relacionado con** | SPEC RN-05, RN-12, RN-15, RN-20, RN-21, RN-27; [ADR-001](ADR-001-limites-de-servicios.md); [ADR-003](ADR-003-persistencia.md); [ARCHITECTURE §6–8](../ARCHITECTURE.md#6-comunicaciones-síncronas-y-asíncronas) |

## Contexto

Con los límites de ADR-001, hay interacciones que cruzan servicios:

- Reservar exige saber si la membresía está vigente (RN-12) — la respuesta es **necesaria** para decidir.
- Registrar una asistencia debe acreditar puntos **exactamente una vez** (RN-21), aunque benefits esté caído en ese momento.
- Asignar una membresía debe enviar un email **sin bloquear** la operación (RN-05).
- La vista de clases (OpenSearch) debe reflejar los cambios de la agenda.
- Los partners reintentan operaciones y no deben generar efectos duplicados (RN-27).

El riesgo principal es el **dual write**: guardar en la base y publicar en el broker son dos sistemas; si uno falla, quedan inconsistentes.

## Alternativas consideradas

### Alternativa A — Todo síncrono (HTTP)

booking llama a benefits para acreditar; members llama al worker para enviar email.

| Pros | Contras |
|---|---|
| Simple de seguir y depurar | Acopla disponibilidad: si benefits cae, no se puede registrar el ingreso |
| Respuesta inmediata | Reintentos y duplicados quedan a cargo del llamador |
| | El email bloquea el alta de membresía |

### Alternativa B — Todo asíncrono (incluida la verificación de membresía)

booking mantiene una réplica local de vigencias alimentada por eventos de members.

| Pros | Contras |
|---|---|
| booking no depende de members en tiempo de ejecución | Una membresía recién cancelada podría seguir habilitando reservas hasta que llegue el evento |
| | Requiere más eventos y una proyección extra desde el día uno |

### Alternativa C — Híbrido (elegida)

Síncrono solo donde la respuesta es parte de la decisión; eventos para propagar hechos. Publicación con **Transactional Outbox**; consumo **idempotente** con ACK después del efecto.

| Pros | Contras |
|---|---|
| Consistencia fuerte donde importa (membresía al reservar) | booking depende de la disponibilidad de members |
| Desacople temporal para puntos, emails y vista de lectura | Consistencia eventual en puntos y en el listado |
| Sin dual write | Outbox y deduplicación agregan tablas y procesos |

### Alternativa D — Publicar directo al broker después del commit (sin outbox)

| Pros | Contras |
|---|---|
| Menos piezas | Si el proceso cae entre el commit y la publicación, el evento se pierde (puntos que nunca llegan) |

## Decisión

Adoptamos la **alternativa C**.

### Qué es síncrono

| Interacción | Timeout | Reintentos | Ante falla |
|---|---|---|---|
| Frontend → gateway → servicio | 3 s lecturas / 5 s escrituras (por ruta) | Ninguno en el gateway; el cliente reintenta con la misma `Idempotency-Key` | `504`/`502` RFC 7807 |
| Partner → gateway → benefits | 5 s | El partner, con la misma `Idempotency-Key` | RFC 7807 |
| booking → members: vigencia de membresía | 800 ms | 1 reintento con backoff 100 ms (GET idempotente) | Circuit breaker (abre con 50 % de errores en 20 requests, semiabierto a los 10 s). **Falla cerrado**: `503 DEPENDENCIA_NO_DISPONIBLE`, no se reserva |
| booking → members: datos de alumnos para listados | 800 ms | 1 | Degrada: devuelve IDs sin nombre |
| training → members: asignación | 800 ms | 1 + caché 60 s | Falla cerrado |
| benefits → members: validar alumno al vincular | 800 ms | 1 | `503` |

Reglas generales: todo cliente HTTP usa el `context` de la request (el timeout total nunca supera el del gateway), propaga `traceparent` y `X-Correlation-Id`, y solo reintenta operaciones idempotentes.

### Qué es asíncrono

| Evento | Productor → Consumidor | Por qué asíncrono |
|---|---|---|
| `membresia.activada` | members → notification-worker | El email es lento y externo; no debe bloquear ni revertir la membresía (RN-05) |
| `membresia.recordatorio_vencimiento`, `membresia.advertencia_vencimiento` | members → notification-worker | Los avisos de vencimiento son asíncronos y no deben bloquear el vencimiento ni depender de un partner (RN-41) |
| `asistencia.registrada` | booking → benefits | Registrar el ingreso no debe depender de benefits; +500 puntos con consistencia eventual (CL-15) |
| `inasistencia.registrada` | booking → benefits | Aplicar la penalización de hasta -100 puntos al cerrar una clase, sin saldo negativo y con consistencia eventual |
| `clase.actualizada` | booking → booking-indexer | Mantener el modelo de lectura sin cargar la escritura |
| `membresia.cancelada`, `usuario.desactivado` | members → booking | Cancelar reservas futuras (RN-04, RN-34) sin acoplar members a booking |

Topología, envelope y catálogo: [ARCHITECTURE §7–8](../ARCHITECTURE.md#7-catálogo-inicial-de-eventos).

### Outbox

1. El cambio de negocio y la fila de `outbox` se escriben en **la misma transacción**.
2. Un relay de polling (cada 500 ms, lotes de 100, `FOR UPDATE SKIP LOCKED`) publica con publisher confirms y marca la fila como publicada.
3. Garantía resultante: **al menos una vez**, sin pérdidas.

### Consumo e idempotencia

- Cada consumidor guarda el `event_id` en la misma transacción que su efecto; si ya existe, hace ACK sin repetir el efecto.
- **ACK después del efecto.** `prefetch = 10`.
- Errores transitorios → colas de reintento con demora (10 s, 1 min, 10 min) mediante TTL + dead-letter; al 4.º fallo o ante error permanente → DLQ.
- Orden: no garantizado; `clase.actualizada` lleva `version` y el indexer descarta versiones viejas. benefits no depende del orden (cada asistencia es independiente).

### Idempotencia HTTP

- `Idempotency-Key` en escrituras sensibles del frontend (reservar, cancelar, canjear, registrar ingreso) — retención 24 h.
- `Idempotency-Key` **obligatoria** en la API de partners — retención ≥ 30 días, por partner (RN-27).
- Misma clave + mismo request → misma respuesta; misma clave + request distinto → `409 CONFLICTO_IDEMPOTENCIA`.

## Consecuencias

### Positivas

- Ningún evento se pierde por caídas entre la base y el broker.
- Una caída de benefits o del SMTP no afecta reservas, asistencias ni membresías.
- Reintentos seguros de punta a punta (cliente, partner, broker).
- La traza distribuida cruza la frontera asíncrona (`traceparent` en el envelope).

### Negativas

- Reservar depende de members-service en línea.
- Puntos y vista de clases con demora (segundos).
- Más piezas: outbox, relay, tablas de deduplicación, colas de reintento y DLQ.

### Riesgos aceptados

| Riesgo | Probabilidad | Impacto | Mitigación |
|---|---|---|---|
| members caído bloquea reservas | Baja | Alto | Circuit breaker, health checks, alerta en Grafana |
| Email duplicado si el worker cae entre enviar y registrar | Baja | Bajo | Estado `PROCESANDO` previo; aceptado (SMTP no es transaccional) |
| Acumulación en DLQ sin atención | Media | Medio | Métrica y alerta de mensajes en DLQ; comando `make dlq-replay` |
| Latencia del outbox por polling | Alta | Bajo | 500 ms es aceptable para el dominio; CDC como mejora futura |

## Cómo se verificará

- **Outbox:** test de integración que fuerza un fallo del broker después del commit → al recuperarse, el evento se publica.
- **Idempotencia de consumidores:** reentregar el mismo `asistencia.registrada` 3 veces → un solo movimiento (+500); reentregar `inasistencia.registrada` no duplica la penalización.
- **Reintentos y DLQ:** consumidor que falla siempre → el mensaje pasa por las 3 colas de reintento y termina en la DLQ.
- **Circuit breaker:** con members detenido, `POST /reservas` responde `503` en < 1 s y no crea reservas.
- **Partners:** el mismo `Idempotency-Key` enviado 10 veces en paralelo → un solo movimiento, 10 respuestas iguales.
- **Trazas:** en Jaeger, una reserva con asistencia muestra la traza gateway → booking → (outbox) → benefits.
- **Métricas:** lag de colas, mensajes en DLQ y filas pendientes en outbox visibles en Grafana.
