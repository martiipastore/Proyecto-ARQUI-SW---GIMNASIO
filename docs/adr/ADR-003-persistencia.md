# ADR-003 — Persistencia por servicio (versión inicial)

| Campo | Valor |
|---|---|
| **Estado** | Aceptado |
| **Fecha** | 2026-10-07 |
| **Decisión del TP** | D3 |
| **Autores** | Equipo del TPI |
| **Relacionado con** | SPEC RN-13, RN-14, RN-21 a RN-27, RN-31, RN-39, RN-40; [ADR-001](ADR-001-limites-de-servicios.md); [ADR-005](ADR-005-comunicacion.md) |

## Contexto

Cada servicio es dueño de sus datos (ADR-001). Hay que elegir el almacenamiento de cada uno según sus **patrones de acceso** y sus **invariantes**:

- **Reservas:** concurrencia alta sobre una misma clase (último lugar, RN-14), unicidad (RN-13), máximo un turno de Musculación por día (RN-39), sin superposición (RN-40). Requiere transacciones y restricciones fuertes.
- **Puntos:** ledger inmutable, saldo nunca negativo con débitos concurrentes (RN-23), idempotencia por partner (RN-27). Requiere transacciones y restricciones fuertes.
- **Búsqueda de clases disponibles:** lectura frecuente con filtros (actividad, fecha, franja, disponibilidad), tolerante a pequeños retrasos.
- **Planes de entrenamiento:** documento jerárquico (plan → días → ejercicios) que se lee y escribe entero; esquema que puede variar.
- **Mediciones:** append-only, consulta por alumno ordenada por fecha (RN-31).
- **Notificaciones:** registro de mensajes procesados para idempotencia.

Además se pide **Database per Service** con una sola instancia de PostgreSQL en local.

## Alternativas consideradas

### Alternativa A — PostgreSQL para todo

| Pros | Contras |
|---|---|
| Una sola tecnología que operar | Planes jerárquicos se modelan en 3 tablas o en JSONB; más mapeo |
| Transacciones en todos los servicios | No demuestra persistencia políglota (objetivo didáctico) |
| | Búsqueda facetada sobre clases cargaría la base de escritura |

### Alternativa B — MongoDB para todo

| Pros | Contras |
|---|---|
| Modelo flexible | Las invariantes de cupo y saldo dependen de transacciones multi-documento y de restricciones que Mongo ofrece con menos garantías y más complejidad (replica set obligatorio) |
| | Sin restricciones `CHECK`, índices parciales únicos con menor expresividad para "una reserva CONFIRMADA por alumno y clase" |

### Alternativa C — Persistencia políglota por servicio (elegida)

PostgreSQL donde hay invariantes transaccionales; MongoDB donde el dato es un documento; OpenSearch como modelo de lectura; Redis como caché.

| Pros | Contras |
|---|---|
| Cada servicio usa el almacenamiento que mejor encaja con su acceso | Más tecnologías que operar y aprender |
| Invariantes críticas garantizadas por la base | Consistencia eventual entre escritura (Postgres) y lectura (OpenSearch) |
| Lectura escalable sin cargar la base de reservas | |

### Alternativa D — Una instancia de PostgreSQL por servicio también en local

| Pros | Contras |
|---|---|
| Aislamiento físico real | Más memoria y contenedores en la máquina de cada integrante |
| | No agrega valor funcional en local |

## Decisión

| Servicio | Almacén | Base / índice | Por qué |
|---|---|---|---|
| members-service | PostgreSQL | `members_db` | Datos relacionales (usuario–rol–membresía–asignación), unicidad de DNI/email, no solapamiento de membresías |
| booking-service | PostgreSQL (escritura) + OpenSearch (lectura) | `booking_db` + índice `clases` | Transacciones y restricciones para el cupo; búsqueda rápida y filtrable para el listado |
| benefits-service | PostgreSQL | `benefits_db` | Ledger con `CHECK (saldo >= 0)`, unicidad de idempotencia, transacciones con bloqueo de fila |
| training-service | MongoDB | `training_db` | Planes como documento único; mediciones append-only |
| notification-worker | MongoDB | `notifications_db` | Registro simple de mensajes procesados con índice único por `event_id` |
| api-gateway | Redis | prefijo `gw:rl:*` | Token bucket atómico (script Lua) |
| Varios | Redis | prefijo por servicio | Cache-aside de catálogos |

**Variante de Database per Service:** una instancia de PostgreSQL con **una base por servicio** y **un usuario por servicio** con permisos solo sobre su base. Ningún servicio hace consultas a otra base; los datos ajenos se obtienen por API o eventos.

### Patrones de acceso y cómo se garantizan las reglas

| Regla | Mecanismo de persistencia |
|---|---|
| RN-14 cupo bajo concurrencia | En la transacción de reserva: `UPDATE clases SET ocupacion = ocupacion + 1 WHERE id = $1 AND estado = 'PROGRAMADA' AND ocupacion < capacidad` → si afecta 0 filas, `SIN_CUPO`. Luego `INSERT` de la reserva. Todo en `READ COMMITTED` con el bloqueo de fila que toma el `UPDATE`. |
| RN-13 una reserva CONFIRMADA por alumno y clase | Índice único parcial `(alumno_id, clase_id) WHERE estado = 'CONFIRMADA'`. |
| RN-39 un turno de Musculación por día | Índice único parcial `(alumno_id, fecha) WHERE actividad = 'MUSCULACION' AND estado IN ('CONFIRMADA','ASISTIDA')` (columna desnormalizada en la reserva). |
| RN-40 sin superposición | Restricción de exclusión con `tstzrange(inicio, fin, '[)')` y `btree_gist`: `EXCLUDE USING gist (alumno_id WITH =, horario WITH &&) WHERE (estado = 'CONFIRMADA')`. El rango semiabierto permite clases contiguas (20:00–20:00). |
| RN-17 cancelación libera cupo una sola vez | `UPDATE reservas SET estado='CANCELADA' WHERE id=$1 AND estado='CONFIRMADA'`; solo si afectó 1 fila se decrementa la ocupación. |
| RN-21 una asistencia por reserva | `UNIQUE (reserva_id)` en asistencias + transición condicional del estado de la reserva. |
| RN-22/23 ledger y saldo | `movimientos_puntos` append-only (sin `UPDATE` salvo el estado `REVERTIDO`); `cuentas.saldo` con `CHECK (saldo >= 0)` actualizado en la misma transacción. |
| RN-21 puntos una vez por asistencia | `UNIQUE (origen_asistencia_id)` en movimientos + `processed_messages`. |
| RN-27 idempotencia de partner | `UNIQUE (partner_id, idempotency_key)` en `operaciones_partner`, con hash del request y respuesta guardada. |
| RN-31 mediciones inmutables | Colección `mediciones` solo con inserciones; el repositorio no expone update/delete. |
| Outbox | Tabla `outbox` en cada base que publica, escrita en la misma transacción que el cambio. |

### Modelo de lectura (OpenSearch)

- Índice `clases` con un documento por clase: actividad, fecha, inicio/fin, franja, profesor, capacidad, ocupación, estado, `version`.
- Lo mantiene `booking-indexer` consumiendo `clase.actualizada`; upsert solo si `version` es mayor a la indexada.
- **Reconstruible:** `booking-indexer --reindex` recorre `booking_db` y regenera el índice con alias (cero downtime).
- Solo sirve **listados y búsquedas**. Ninguna decisión de negocio se toma leyendo OpenSearch.

### MongoDB

- Se levanta como **replica set de un nodo** para habilitar transacciones si se necesitan (por ejemplo, finalizar el plan VIGENTE anterior y crear uno nuevo).
- Índices: `planes_entrenamiento (alumno_id, estado)`, índice único parcial `(alumno_id) WHERE estado = 'VIGENTE'` para garantizar un único plan vigente; `mediciones (alumno_id, fecha)`; `consultas_nutricionales (nutricionista_id, estado, enviada_en)`; `mensajes (event_id)` único.

### Migraciones

- Postgres: archivos SQL versionados en `services/<svc>/migrations/`, aplicados por el servicio al arrancar (o por un job de Compose).
- Mongo: creación de índices idempotente al arrancar.

## Consecuencias

### Positivas

- Las reglas más delicadas del SPEC las garantiza la base de datos, no solo el código.
- Los listados de clases no compiten con las escrituras de reservas.
- Separar instancias de Postgres en la nube es cambiar la cadena de conexión.

### Negativas

- Cuatro tecnologías de datos (Postgres, Mongo, Redis, OpenSearch) para operar y probar.
- La vista de clases puede mostrar ocupación desactualizada por unos segundos.
- Restricciones avanzadas (`EXCLUDE`, índices parciales) atan booking a PostgreSQL.

### Riesgos aceptados

| Riesgo | Probabilidad | Impacto | Mitigación |
|---|---|---|---|
| Instancia única de Postgres como punto único de falla | Media | Alto | Aceptado en local y en la VM del TP; volúmenes persistentes y backups simples |
| Desfase OpenSearch ↔ Postgres | Media | Bajo | Validación siempre en Postgres; reindexado completo disponible; métrica de lag |
| Contención en la fila de una clase muy demandada | Media | Medio | Transacciones cortas; prueba k6 sobre "último lugar" |
| Pérdida de datos de Redis | Media | Bajo | Solo caché y contadores; nada que no se pueda recalcular |

## Cómo se verificará

- **Tests de integración con testcontainers-go** (Postgres y Mongo reales):
  - 50 goroutines reservan el último lugar → exactamente 1 éxito, ocupación = capacidad.
  - Reservas superpuestas y segundo turno de Musculación del día → rechazados por la base aun saltando la validación de aplicación.
  - Débitos concurrentes que exceden el saldo → saldo final ≥ 0.
  - Mismo `event_id` de asistencia procesado dos veces → un solo movimiento.
- **Permisos:** test que verifica que el usuario de cada servicio no accede a otra base.
- **CQRS:** test de que un `clase.actualizada` con `version` menor no pisa el documento.
- **Carga (k6):** p95 de `GET /clases` < 200 ms y de `POST /reservas` < 500 ms con 2 instancias de booking (umbrales iniciales).
