# Etapa 8 — Búsqueda indexada de clases

**Estado:** verificación técnica local cerrada el 10/11/2026. El E2E aprobó
**44 escenarios**: los 38 de las etapas 4 a 7 y 6 nuevos. Evidencia y mediciones
en [search-e2e.json](../evidence/entrega-final/etapa8/search-e2e.json). Falta la
revisión de otro integrante. No se publicó en Vercel.

Historias y reglas: HU-13 (búsqueda), RN-14 (el cupo lo decide PostgreSQL).
Diseño: [ADR-014](../adr/ADR-014-busqueda-clases.md), D-45. Pendientes resueltos
por el equipo el 10/11/2026 en **D-87**: reindexado con alias y repaso,
profundidad máxima de 10.000 resultados y `503` explícito sin OpenSearch.

## 1. Flujo

```text
escritura en clases ──trigger──▶ outbox (clase.actualizada, versión N)   ← misma transacción
        relay (etapa 5) ──▶ RabbitMQ gym.events ──▶ booking-indexer.clase-actualizada
        booking-indexer ──PUT version=N, version_type=external──▶ OpenSearch alias clases-read
GET /classes ──▶ booking-service ──▶ OpenSearch (búsqueda) + PostgreSQL (reservada_por_mi)
POST /bookings ──▶ booking-service ──▶ PostgreSQL (cupo, estado y reglas con la fila bloqueada)
```

1. **Versión por clase.** La migración `002_busqueda_clases.sql` agrega
   `clases.version`. Un trigger la incrementa en cada `UPDATE`.
2. **Captura de cambios por trigger.** Otro trigger escribe `clase.actualizada` en
   el outbox con el estado completo de la clase, en la misma transacción
   (CLAUDE.md regla 6). Se eligió trigger y no código en cada caso de uso: así
   ningún camino que toque una clase puede olvidarse de publicar (generar,
   reservar, cancelar reserva, cambiar capacidad, cancelar clase, cierre
   automático y fixtures SQL). Renombrar una actividad vuelve a publicar sus clases.
3. **Cola durable.** El relay de Booking declara la cola del indexer aunque el
   indexer esté apagado, así que los eventos esperan sin perderse.
4. **Indexer.** `booking-indexer` consume con el consumidor común de la etapa 5:
   ACK manual, reintentos a los 10 s, 1 min y 10 min, y DLQ. Escribe con
   `version=N&version_type=external`: OpenSearch rechaza una versión menor o
   igual a la guardada, y ese rechazo se confirma como descartado. Por eso las
   reentregas y los eventos fuera de orden no pisan datos nuevos. Si OpenSearch
   no responde, reintenta dentro del mismo mensaje hasta 12 s antes de mandarlo a
   la cola de reintentos.
5. **Índice.** Tiene mapping estricto (`dynamic: strict`) y el Compose desactiva
   `action.auto_create_index`. El indexer crea el primer índice y el alias antes
   de empezar a consumir. Así nunca se crea un índice con tipos adivinados.
6. **Búsqueda.** Mantiene la semántica de la etapa 4: rango `[desde, hasta)` sobre
   `inicio`, `actividad_id`, `franja` como hora de inicio local `HH:MM` y
   `solo_disponibles` (PROGRAMADA, futura y con lugares). Orden por `inicio`
   ascendente e `id` como desempate. El E2E compara cada filtro con la misma
   consulta en PostgreSQL.

## 2. Reindexado (D-87)

`booking-indexer -reindex` (o el binario con ese flag) reconstruye el índice sin
pausar Booking ni el indexer:

1. crea `clases-<timestamp>` y carga todas las clases de booking_db en lotes de
   500 con `_bulk` y versión externa;
2. mueve el alias `clases-read` en una sola acción `_aliases`: desde ese momento
   el indexer escribe en el índice nuevo;
3. repasa todas las clases otra vez. Lo que cambió durante la carga entra con
   su versión nueva y lo que ya estaba más nuevo se descarta (conflicto 409 esperado);
4. verifica la convergencia: misma cantidad y misma suma de versiones que
   booking_db, con reintentos mientras siguen llegando eventos. Recién entonces
   borra el índice anterior.

El test de integración lo ejercita con 1.200 clases y un escritor concurrente.
El E2E lo ejecuta con el indexer consumiendo al mismo tiempo.

## 3. Degradación y revalidación

- Booking clasifica OpenSearch como dependencia **parcial** (ADR-007). Si se cae,
  `GET /classes` responde `503 DEPENDENCIA_NO_DISPONIBLE` y readiness queda
  `degraded`. Reservar, cancelar, ingresar, ver el detalle de clase y escribir
  siguen funcionando contra PostgreSQL.
- Para el indexer, OpenSearch es **crítico** (sin él no puede trabajar) y la cola
  es parcial.
- Al volver OpenSearch, los eventos retenidos se aplican y la búsqueda converge.
  Si un evento agotara sus reintentos y fuera a la DLQ, la reconciliación es
  correr `-reindex`.
- La búsqueda nunca decide una reserva. En el E2E se cancela una clase con el
  indexer apagado: la búsqueda la sigue mostrando PROGRAMADA, pero la reserva
  se rechaza con `CLASE_CANCELADA` porque se valida en PostgreSQL (RN-14).
- `reservada_por_mi` (CA-13.4) se consulta en PostgreSQL solo para los ids de la
  página. Es dato propio del alumno y no se indexa.

## 4. Medición del retraso (ADR-014: < 2 s)

En el E2E se cambió cinco veces la capacidad de una clase y se midió el tiempo
desde la respuesta `200` de la escritura hasta verla en `GET /classes` por el
gateway, consultando cada 50 ms.

| Medida | Resultado |
|---|---|
| Máximo escritura → búsqueda | 1,07 s |
| Promedio | 1,02 s |

Valores de la primera corrida completa; la evidencia JSON guarda los de la última.
El retraso está dominado por el refresco de 1 s de OpenSearch. El indexer expone
en `/metrics` el retraso desde el commit (`occurred_at` con `clock_timestamp()`)
hasta indexar: `clase_index_lag_seconds_last|max|avg`, aplicados, descartados,
reintentos y DLQ. El máximo incluye el backlog de eventos encolados antes de que
existiera el indexer, por eso el informe guarda el último valor por separado.
Son mediciones locales en una sola máquina, no una garantía de producción.

## 5. Configuración local

```text
BOOKING_ENABLED=true
BOOKING_MESSAGING_ENABLED=true
BOOKING_SEARCH_ENABLED=true      # GET /classes desde OpenSearch
BOOKING_INDEXER_ENABLED=true     # consumidor del booking-indexer
docker compose --env-file .env -f deploy/docker-compose.yml --profile search up -d --build --wait
```

OpenSearch 2.17.1 corre con el perfil `search`: 512 MB de heap, nodo único, sin
plugin de seguridad, solo en `127.0.0.1:9200`. Nadie más que booking-service y el
indexer accede al motor. Los flags están apagados por defecto y sin ellos la
búsqueda sigue en PostgreSQL como en la etapa 4. Reindexar en Compose:
`docker compose ... --profile search run --rm booking-indexer -reindex` (el entrypoint
ya es el binario del indexer).

## 6. Verificación

```text
go run ./tests/e2e/identity/main.go -search
```

Agrega a las etapas 4 a 7: reindexado con alias y convergencia; filtros, orden,
paginación y `reservada_por_mi` iguales a PostgreSQL, más el límite de
profundidad; retraso medido; evento viejo descartado; búsqueda atrasada que no
decide la reserva; y caída de OpenSearch con 503, escrituras que siguen y recuperación.

Tests:

- `TestOpenSearchReal` (OpenSearch 2.17.1 real con testcontainers): orden y
  desempate, paginación, filtros, versión vieja y repetida descartadas,
  reindexado de 1.200 clases con escritor concurrente y motor caído.
- `TestBusquedaPostgres`: versión y evento en la misma transacción, rollback sin
  evento, renombre que republica, unicidad de eventos de asistencia, marcas,
  recorrido y resumen.
- Unitarios del adaptador AMQP (esquema, DLQ, espera) y de aplicación (marcas,
  profundidad, 503 sin respaldo).

Resultados del 10/11/2026: `go vet` y `go test ./...` de booking-service con
Docker aprobados y E2E de 44 escenarios aprobado.

Se corrigió además un error que ya tenía la prueba E2E de CB-04: la fixture usaba
`CURRENT_DATE` en UTC y entre las 21 y las 24 de Argentina la membresía «vencida
ayer» vencía en realidad hoy. Ahora usa la fecha de `America/Argentina/Buenos_Aires`.

## 7. Límites

- `traceparent` sigue vacío en el outbox; la propagación es de la etapa 14.
- Un evento que agota los reintentos (más de ~11 min con OpenSearch caído) va a la
  DLQ y se reconcilia con `-reindex`. No hay reproceso automático de la DLQ.
- No se publicó en Vercel: OpenSearch y RabbitMQ no forman parte del despliegue
  actual (ADR-021).
