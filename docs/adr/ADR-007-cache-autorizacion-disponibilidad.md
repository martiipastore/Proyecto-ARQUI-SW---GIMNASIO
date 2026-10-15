# ADR-007 — Caché, autorización y disponibilidad degradada

| Campo | Valor |
|---|---|
| **Estado** | Aceptado |
| **Fecha** | 2026-10-08 |
| **Decisión del TP** | D7 |
| **Autores** | Equipo del TPI |
| **Relacionado con** | SPEC RN-12, RN-26, RN-30 y RN-34; [ADR-003](ADR-003-persistencia.md); [ADR-005 v2](ADR-005-v2-comunicacion.md) |

## Contexto

Redis puede reducir latencia y carga en consultas repetidas, pero algunos datos determinan si una operación está permitida. Una copia temporalmente vieja de una asignación profesional–alumno, una membresía o una API key revocada puede mantener un permiso que ya no existe.

También necesitamos distinguir entre una dependencia cuya caída inutiliza la responsabilidad principal de un proceso y otra que solo afecta una función. Si toda dependencia se trata como obligatoria, una caída de Redis, SMTP u OpenSearch retira servicios que todavía pueden responder operaciones válidas y amplía el impacto de la falla.

## Alternativas consideradas

### Alternativa A — Cachear todas las lecturas con un TTL corto

Descartada. Mejora latencia y reduce tráfico hacia members, pero crea un período de gracia durante el cual una autorización revocada todavía puede aceptarse. Un TTL de 60 segundos reduce la ventana, pero no elimina la contradicción con RN-30.

### Alternativa B — No usar caché y exigir todas las dependencias en readiness

Descartada. Protege las autorizaciones, pero desaprovecha Redis para catálogos y acopla la disponibilidad completa de cada proceso a funciones parciales. Una falla de SMTP, por ejemplo, no debería impedir que el worker conserve mensajes para reintento.

### Alternativa C — Cachear solo datos no críticos y clasificar dependencias (elegida)

Los datos que conceden permisos siempre se obtienen de su fuente vigente. La caché se limita a información cuya antigüedad temporal no habilita acciones indebidas. Cada proceso clasifica sus dependencias como críticas o parciales y expone la diferencia en readiness.

## Decisión

### Datos que pueden cachearse

Se usa cache-aside para:

| Dato | Dueño / consumidor | TTL inicial | Invalidación |
|---|---|---:|---|
| Catálogo de actividades | booking | 10 min | Al modificar una actividad |
| Catálogo de beneficios | benefits | 5 min | Al modificar un beneficio |
| Tipos de membresía | members | 1 h | Al modificar un tipo |

Si Redis no está disponible, el servicio consulta la fuente de verdad y registra la falla. Los TTL son valores iniciales que se ajustarán con métricas de aciertos, latencia y carga.

### Datos que no pueden cachearse

No se cachean los datos que deciden autorización:

- vigencia de una membresía para reservar o ingresar;
- asignación profesional–alumno;
- validez de una API key de partner.

Training consulta a members en cada operación protegida. Si members no puede confirmar la asignación después del timeout y el reintento definidos, training responde `503 MEMBERS_NO_DISPONIBLE` y no ejecuta la operación. Una respuesta negativa vigente produce `403 SIN_PERMISO`. Esta política falla cerrada y hace efectiva una revocación en la siguiente solicitud.

Los listados que consultan datos ajenos solo para enriquecer una vista pueden degradar su respuesta. Por ejemplo, booking puede devolver identificadores de alumnos sin sus nombres si members está temporalmente caído.

### Liveness y readiness

`/health/live` solo indica que el proceso puede atender HTTP y nunca consulta sistemas externos.

En `/health/ready`, cada control declara si es crítico:

- una dependencia **crítica** caída produce `not_ready` y HTTP 503;
- una dependencia **parcial** caída produce `degraded` y HTTP 200;
- todas las dependencias disponibles producen `ready` y HTTP 200.

La clasificación inicial es:

| Proceso | Críticas | Parciales |
|---|---|---|
| members-service | PostgreSQL | RabbitMQ, Redis |
| booking-service | PostgreSQL | members-service, OpenSearch, RabbitMQ, Redis |
| booking-indexer | RabbitMQ, OpenSearch | PostgreSQL para reindexado |
| benefits-service | PostgreSQL | members-service, RabbitMQ, Redis |
| training-service | MongoDB | members-service |
| notification-worker | MongoDB, RabbitMQ | SMTP |
| api-gateway | Ninguna | Redis, servicios de destino |

La clasificación de una dependencia como parcial no oculta la falla. El estado degradado aparece en `/health/ready`, `/api/v1/status`, métricas y logs, y el endpoint que la necesita aplica su respuesta específica.

### Estado agregado

El gateway consulta los readiness en paralelo:

- devuelve `200 ready` si todos están listos;
- devuelve `200 degraded` si al menos uno está degradado y ninguno está no disponible;
- devuelve `503 not_ready` si algún proceso responde `not_ready` o resulta `unreachable`.

El endpoint público no revela mensajes internos de conexión ni secretos.

## Consecuencias

### Positivas

- Revocar una asignación impide la siguiente operación protegida.
- La caída de una dependencia parcial no retira capacidades que todavía funcionan.
- El estado operativo distingue una degradación de la incapacidad de cumplir la responsabilidad principal.
- Redis sigue aportando rendimiento sin convertirse en fuente de permisos.

### Negativas

- Las operaciones protegidas agregan una llamada síncrona a members y dependen de su latencia.
- La respuesta de readiness y el estado agregado incorporan un estado adicional.
- Cada endpoint debe definir cómo responde cuando falla una dependencia parcial.
- La clasificación debe revisarse cuando cambien las responsabilidades de un proceso.

## Cómo se verificará

- Revocar una asignación y repetir inmediatamente una operación protegida produce `403` y no modifica datos.
- Interrumpir members durante una operación protegida de training produce `503` y no modifica datos.
- Con Redis caído, los catálogos se leen desde su fuente, el proceso informa `degraded` y readiness responde 200.
- Con OpenSearch caído, booking informa `degraded`; las escrituras que usan PostgreSQL siguen disponibles y la búsqueda afectada responde según su contrato.
- Con PostgreSQL caído en booking, readiness responde 503 y estado `not_ready`.
- Con SMTP caído, notification-worker conserva/reintenta mensajes, informa `degraded` y no pierde la notificación.
- `/api/v1/status` responde 200 ante degradaciones parciales y 503 ante un proceso `not_ready` o `unreachable`, sin exponer errores internos.
