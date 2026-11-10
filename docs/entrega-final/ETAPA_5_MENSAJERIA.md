# Etapa 5 — Mensajería y notificaciones

Etapa del [plan final](PLAN_ENTREGA_FINAL.md), iniciada el 10/11/2026 después de
cerrar la verificación técnica de Booking. **Estado: verificación técnica local
cerrada el 10/11/2026; integración y revisión de otro integrante pendientes.** RN-05, HU-03/04/12,
ADR-003 y ADR-005 v2; topología de ARCHITECTURE §8.

El E2E final aprobó **26 escenarios**, con `passed: true` en
[messaging-e2e.json](../evidence/entrega-final/etapa5/messaging-e2e.json).
Aprobaron vet y tests de Members, notification-worker y `pkg`; el test adicional
de retención TTL sin destino también pasó con RabbitMQ real. Members y el worker
compilan con `GOWORK=off`; Compose valida con `.env.example`.

## Implementación

- Members conserva la escritura transaccional de `outbox_membresias`, incluido
  el evento de asignación o renovación. El relay bloquea hasta 100 filas con
  `FOR UPDATE SKIP LOCKED`, publica mensajes persistentes con publisher confirms
  y marca `publicado_en` solo tras confirmación. Son posibles reentregas con el
  mismo `event_id` si falla el commit después de publicar.
- El relay declara la topología antes de publicar, aunque el worker esté apagado.
  Solo publica `membresia.activada`: los eventos de cancelación permanecen
  pendientes hasta implementar su consumidor en Booking.
- El worker consume con ACK manual y prefetch 10. Una notificación es un único
  documento MongoDB con `_id=event_id`, huella del contenido normalizado, estado,
  cantidad de intentos, resultados de cada envío y marcas temporales. Las escrituras usan majority.
- La reserva atómica del documento evita dos envíos concurrentes. El lease de
  20 segundos supera el presupuesto de procesamiento de 15 segundos y vence si
  muere el worker. SMTP tiene un límite de 10 segundos. No se borran las claves
  de deduplicación al terminar.
- Los errores transitorios pasan por las colas TTL de 10 segundos, 1 minuto y
  10 minutos. El cuarto error, JSON inválido o versión desconocida pasa a DLQ.
  El ACK del original ocurre después de confirmar el destino del reintento/DLQ.
  Las colas de reintento también son quorum y usan dead lettering
  `at-least-once` con `reject-publish`, para conservar el mensaje si vence el TTL
  mientras el destino está indisponible. Ver [garantías de RabbitMQ 3.13](https://www.rabbitmq.com/docs/3.13/quorum-queues#dead-lettering).
- MongoDB guarda diagnóstico de mensajes fallidos por hash del cuerpo, sin
  copiar información personal al diagnóstico ni a los logs.
- RabbitMQ es parcial en readiness de Members y crítico en el worker. La caída
  del broker no revierte una membresía ni impide confirmar una escritura local.

El cliente [amqp091-go](https://github.com/rabbitmq/amqp091-go), versión fijada
1.10.0, conecta con el RabbitMQ ya elegido. No incorpora un broker ni un patrón
nuevo. MongoDB usa atomicidad de documento; no exige cambiar el Compose a replica set.

## Activación local

En `.env`, con identidad configurada como en las etapas anteriores:

```text
MEMBERS_IDENTITY_ENABLED=true
MEMBERS_MESSAGING_ENABLED=true
NOTIFICATION_MESSAGING_ENABLED=true
NOTIFICATION_SMTP_ALLOW_PLAINTEXT=true
```

El último flag se limita a Mailpit local sin credenciales. Otros servidores
requieren STARTTLS; el worker admite `NOTIFICATION_SMTP_USER` y
`NOTIFICATION_SMTP_PASSWORD` al ejecutar el binario. Compose forma las URLs AMQP
con las credenciales existentes; nunca deben copiarse al repositorio.

## Verificación

Desde la raíz con Docker operativo:

```text
go run ./tests/e2e/identity/main.go -messaging
```

El comando repite identidad, membresías y Booking, y agrega RabbitMQ 3,
MongoDB 7 y el worker con nombres exclusivos. Comprueba outbox, SMTP real,
reentrega, persistencia durante la caída del broker, recuperación, caída SMTP,
reintento durable, mensaje inválido y DLQ. Solo detiene y elimina sus propios
contenedores. Evidencia: `docs/evidence/entrega-final/etapa5/messaging-e2e.json`.

También ejecutar `go vet ./...` y `go test -count=1 -timeout 15m ./...` en Members
y notification-worker, y en `pkg`. `TestNotificacionDurableMongo` usa MongoDB real
y cubre recuperación SMTP, replay semántico, contenido distinto con el mismo ID
y exclusión de procesamiento simultáneo. `TestMensajesInvalidos` cubre el límite
de entrada antes de invocar persistencia o correo.

`TestReintentoRetieneMensajeSinDestino` elimina temporalmente el binding,
deja vencer el TTL y comprueba la entrega después de restaurarlo. Usa RabbitMQ
real; solo su fixture acorta el TTL y el intervalo interno de re-publicación.
El broker local consultado usa 180 segundos para reintentar dead lettering sin
confirmación: la recuperación de un destino no garantiza entrega inmediata.

Durante la validación hubo ejecuciones fallidas por arranque y recuperación del
broker, preparación de PostgreSQL y replay con otro offset. Se corrigieron y el
E2E final completo pasó. El plazo de reinicio es de 2 minutos y el broker de prueba
usa dos schedulers para reducir contención; el stack habitual no se modifica.

La revisión detectó y corrigió dos problemas de la verificación: esperar el
servidor TCP definitivo de PostgreSQL, evitando el servidor temporal de initdb,
y comprobar el replay por el contador del consumidor en vez de una fotografía
de la cola. Esta última comprobación descubrió una diferencia real de huellas
cuando el mismo instante RFC3339 usaba otro offset; ahora se normaliza a UTC y
el test de MongoDB incluye ese replay.

El límite de cuatro intentos se prueba con un mensaje sintético cuyo header
indica tres fallos anteriores, manteniendo los plazos reales de las colas del
E2E. El cuarto fallo SMTP termina en DLQ y queda registrado con `intentos: 4`.

## Observación y límites

- `http://localhost:8085/metrics`: mensajes procesados, reintentos y DLQ de esta
  instancia. Se reinician con el proceso; los estados e intentos duran en MongoDB.
- RabbitMQ Management, `http://localhost:15672`: backlog de la cola principal,
  las tres colas de reintento y `notification.membresia-activada.dlq`.
- En `members_db`, el backlog propio se obtiene filtrando
  `outbox_membresias WHERE publicado_en IS NULL AND event_type='membresia.activada'`.
  Los otros tipos siguen pendientes por diseño, no deben contarse como falla del relay.
- El efecto lógico está deduplicado. SMTP no ofrece transacción distribuida ni
  una clave idempotente: una caída después de aceptar DATA y antes de marcar
  ENVIADA puede repetir el correo físico. Se conserva el mismo Message-ID.
- No se implementan aquí recordatorios de vencimiento, consumo de cancelaciones
  en Booking ni puntos por asistencia. Los puntos son la etapa 6. Las cancelaciones
  y la protección de baja del profesor siguen pendientes del cierre funcional.
- El envelope incluye productor; correlación y traceparent están vacíos cuando el
  outbox histórico no conserva esos datos. Propagación de trazas pendiente en etapa 14.
- No hay borrado automático de filas publicadas ni herramienta de replay de DLQ.
  Se documentan como deuda operativa; no se simulan comandos inexistentes.
- Este trabajo es local: los flags de mensajería siguen apagados por defecto y
  no modifica ni publica el despliegue de Vercel.
