# CLAUDE.md

Contexto y reglas para asistentes de IA (Claude u otros) que trabajen en este repositorio. Leelo completo antes de proponer o escribir cambios.

## Proyecto

Sistema integral de gestión de gimnasio: Trabajo Práctico Integrador de Arquitectura de Software, construido como **microservicios**. Incluye membresías, clases y reservas con cupo, asistencia, planes de entrenamiento y nutrición, y un Club de Beneficios con puntos expuesto a partners de otros grupos.

- **Qué hace el sistema:** [SPEC.md](SPEC.md) (aprobado; fuente de verdad del negocio).
- **Proveedor externo recibido (10/11/2026):** el PDF de MELT es toda la información recibida. Copia y resumen en `docs/entrega-final/ETAPA_13_MELT.md`: catálogo público, registro referido, login y puntos MELT. Falta código de referido definitivo y definir el flujo/servicio consumidor. Preparación documental únicamente; no adelantar la etapa 13, inventar callbacks ni acreditar en el gimnasio los 100 puntos descritos por MELT. ADR-009 sigue propuesto.
- **Etapa 10 de entrega final (10/11/2026):** training-service implementado (Capas, MongoDB, `TRAINING_ENABLED` apagado): planes de entrenamiento y alimenticios con un solo VIGENTE garantizado por índice único parcial, mediciones inmutables con validador `$jsonSchema`, consultas respondidas una vez por actualización condicional. Autorización por rol (gateway `trainingAllowed` + service) y por asignación vigente consultada a Members en cada operación (D-33), con 503 si Members cae. Members agrega `/internal/v1/asignaciones/vigentes`. Pantallas Mi plan, ficha del alumno y bandeja. E2E `-training` de 27 escenarios. Ver `docs/entrega-final/ETAPA_10_TRAINING.md`. Próxima: etapa 11 (frontend completo). Revisión pendiente; sin publicación.
- **Etapa 9 de entrega final (10/11/2026):** caché del catálogo de actividades en Redis (cache-aside, TTL 10 min, invalidación al confirmar; `BOOKING_CACHE_ENABLED` apagado), elegida con mediciones (D-88). Medida A/B/A/B con control: elimina las lecturas a booking_db sin cambio medible de latencia. El gateway usa `proxy.NewTransport` (64 conexiones ociosas por servicio): con el transporte por defecto agotaba puertos y daba 502. go-redis v9 solo en booking. E2E `-cache` de 24 escenarios. Ver `docs/entrega-final/ETAPA_9_CACHE.md`. Revisión pendiente; sin publicación.
- **Etapa 8 de entrega final (10/11/2026):** búsqueda de clases en OpenSearch verificada localmente. Migración 002 de Booking: versión por clase y `clase.actualizada` por trigger al outbox. `booking-indexer` consume con versionado externo sobre el alias `clases-read`; `-reindex` reconstruye sin pausa (D-87). `GET /classes` lee el índice y responde 503 si se cae, sin respaldo; la reserva sigue validándose en PostgreSQL. Retraso medido ~1 s. Flags `BOOKING_SEARCH_ENABLED`/`BOOKING_INDEXER_ENABLED` apagados; OpenSearch con perfil `search` del Compose. E2E de 44 escenarios. Ver `docs/entrega-final/ETAPA_8_BUSQUEDA.md`. Revisión pendiente; sin publicación.
- **Etapa 7 de entrega final (10/11/2026):** alcance reducido D-76 verificado localmente: gestión de integraciones (alta con API key única guardada como hash, permisos, límites, rotación, desactivación, vinculación), cuenta de una persona para el administrador y reversión idempotente como compensatorio en el mismo ledger (migración 007, auditoría inmutable). Pantalla `#/admin/club` y sección Club en la ficha de persona. E2E de 38 escenarios aprobado. Contrato de partners v2 sin cambios. Ver `docs/entrega-final/ETAPA_7_CLUB.md`. Revisión de otro integrante pendiente; sin publicación en Vercel.
- **Etapa 6 de entrega final (10/11/2026):** verificación técnica local cerrada: relay Booking → RabbitMQ, consumidores Benefits, +500 por asistencia y hasta −100 por ausencia (incluido 0), deduplicación por evento y origen, mismo ledger/orden por cuenta e historial propio en `#/app` y `#/cuenta`. E2E de 32 escenarios, vet/tests y build aprobados. Ver `docs/entrega-final/ETAPA_6_BOOKING_BENEFITS.md`. Flags `BOOKING_MESSAGING_ENABLED` y `BENEFITS_MESSAGING_ENABLED` apagados por defecto. Integrada en `main` por PR #20; sin publicación.
- **Entrega final al 10/11/2026:** etapa 4 verificada localmente con 20 escenarios E2E aprobados. Etapa 5 verificada con 26 escenarios E2E y retención TTL sin destino: relay de activación de membresías, notificaciones MongoDB/SMTP, reintentos y DLQ; flags apagados por defecto, sin publicar en Vercel. Ver `docs/entrega-final/ETAPA_5_MENSAJERIA.md`. La etapa 6 también quedó verificada localmente; sigue la etapa 7. El equipo autorizó continuar etapa por etapa después de verificar cada una; no declarar cerradas etapas con pruebas pendientes. No hacer commits, push ni publicación sin pedido explícito.
- **Cómo está construido:** [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) y [docs/adr/](docs/adr/).
- **Etapa actual del Club (09/11/2026):** CB-02/03/04 implementadas y validadas localmente (E2E de 14 escenarios); publicado en Vercel (D-75, ADR-021). Débitos, catálogo y canjes descartados (D-76). CB-07 parcial con la etapa 7 (integraciones, cuentas y reversiones). Pendientes: CB-05, el resto de CB-07 y la regresión de CB-08. Ver [plan ampliado](docs/PLAN_CLUB_BENEFICIOS.md).
- **Plan de etapas del Word v2:** etapas 7 (mensajería), 9, 10 y 11 (booking) y 14 en adelante no están implementadas; las 5, 8 y 12 son parciales. No empezar etapas nuevas sin que el equipo lo pida.
- **Estado histórico de Entrega 2:** etapa 8 en revisión documental y de demostración; ver [revisión](docs/ETAPA_8_REVISION_ENTREGA.md). Etapa 7 integrada por PR #5 (fb2a17c). Etapa 5/D9 sigue pendiente de proveedor; no declarar todo el plan completo. D-61/D-62 aprobadas: validación aislada con restauración y Vite 6.4.3. Sus comprobaciones no validan la ampliación posterior del Club.

## Ampliación vigente del Club — 06/11/2026

El pedido posterior del equipo amplía el alcance mínimo de Entrega 2 sin reiniciar
el proyecto. Seguir [PLAN_CLUB_BENEFICIOS.md](docs/PLAN_CLUB_BENEFICIOS.md), la
[adenda SPEC 0.11](docs/SPEC_CLUB_BENEFICIOS.md) y D-63 a D-77. D-63/D-64/D-65/D-67
reemplazan D-06/D-08/D-28/D-19 y D-75 reemplaza D-41; no volver a restringir el Club
a alumnos con membresía. Registro, integraciones, acceso y membresías tienen
implementación; recompensas y panel siguen pendientes.

ADR-016/017/018/020/022 son propuestas técnicas, no aprobaciones retrospectivas;
ADR-008 v4 (contrato v2) y ADR-021 (Vercel) están aceptados. Seguridad según ADR-006:
JWT RS256 15 minutos, roles de base y bcrypt mínimo 12; logout descarta en cliente
sin revocación v1. Las membresías calculan su vencimiento según el tipo y solo se
asignan a alumnos (D-77). Flags de identidad y registro apagados por defecto en local.
Mantener sincronizados DECISIONES.md y su Word, SPEC, etapas y ADR en cada cambio.

GitHub Actions está bloqueado por facturación desde el 06/11/2026: correr tests y
`go vet` localmente antes de integrar. Las integraciones con testcontainers
necesitan Docker; si el contenedor Ryuk falla al arrancar Docker, repetir con
`TESTCONTAINERS_RYUK_DISABLED=true`. Producción se publica con `vercel deploy --prod`,
no con un push.

## Stack

| Capa | Tecnología |
|---|---|
| Backend | Go 1.22+ con Gin |
| Frontend | React + Vite + TypeScript |
| Datos | PostgreSQL 16 (una instancia, una base por servicio), MongoDB 7, Redis 7, OpenSearch 2 |
| Mensajería | RabbitMQ (exchange topic `gym.events`) |
| Balanceo | Traefik (solo delante de booking-service) |
| Observabilidad | OpenTelemetry, OTel Collector, Jaeger, Prometheus, Loki, Grafana |
| Email local | Mailpit |
| Infra local | Docker Compose + Makefile |
| CI | GitHub Actions |
| Tests | `go test`, testify, testcontainers-go, k6 |

## Servicios

| Servicio | Puerto | Patrón | Datos | Publica | Consume |
|---|---|---|---|---|---|
| `api-gateway` | 8080 | Middlewares | Redis (rate limit) | — | — |
| `members-service` | 8081 | **Capas** | PostgreSQL `members_db` | `alumno.creado`, `membresia.activada`, `membresia.cancelada`, `usuario.desactivado` (los de membresía se guardan en outbox sin publicador; el resto es diseño objetivo) | — |
| `booking-service` (×2 con Traefik desde la etapa de balanceo; hoy ×1) | 8082 | **Hexagonal + CQRS** | PostgreSQL `booking_db` + OpenSearch | `asistencia.registrada`, `inasistencia.registrada`, `clase.actualizada` | `membresia.cancelada`, `usuario.desactivado` |
| `booking-indexer` | 8086 (health y `/metrics`) | Consumidor | OpenSearch alias `clases-read` | — | `clase.actualizada` (etapa 8; `-reindex` reconstruye) |
| `benefits-service` | 8083 | **Hexagonal** | PostgreSQL `benefits_db` | — | `asistencia.registrada`, `inasistencia.registrada` (etapa 6); `alumno.creado` es diseño objetivo |
| `training-service` | 8084 | **Capas** | MongoDB `training_db` | — | — |
| `notification-worker` | 8085 | Consumidor | MongoDB `notifications_db` | — | `membresia.activada` |
| `web` | 5173 | SPA | — | — | — |

Además (aprobado 2026-10-07): members publica `membresia.cancelada` y `usuario.desactivado`, que consume booking (RN-04, RN-34); training consulta asignaciones a members (RN-30); booking consulta datos de alumnos a members para listados (HU-21).

## Estructura de carpetas

```text
go.work                      une todos los módulos Go (cada go.mod usa replace ../../pkg)
.env.example                 variables del stack local (cp .env.example .env; .env nunca se commitea)
gateway/                     api-gateway: internal/{config,proxy,status,middleware,server}
services/<servicio>/         un módulo Go por servicio
  cmd/<binario>/             main.go (solo cableado)
  internal/                  código privado del servicio
  migrations/                SQL versionado (servicios con Postgres, desde la etapa de cada uno)
pkg/                         librerías técnicas compartidas (sin dominio):
                             config, logger, correlation, problem, health, httpx, httpserver
web/                         frontend (src/api/client.ts es el ÚNICO cliente HTTP; habla solo con el gateway)
deploy/                      docker-compose.yml, docker-compose.mock.yml, postgres/init, mongo/init
docs/                        ARCHITECTURE, adr/, contracts/, api/ (OpenAPI interno), events/ (JSON Schema)
tests/load/                  k6
tests/e2e/                   flujos del SPEC §11
```

Todo proceso Go arranca igual (ver cualquier `cmd/*/main.go`): `config.Load()` → `logger.New` → `signal.NotifyContext` → adaptadores → `httpx.NewRouter` + `health.Register` → `httpserver.Run` (graceful shutdown).

### Patrón CAPAS (members, training)

```text
internal/handler/      HTTP (Gin): parseo, validación de formato, mapeo a RFC 7807
internal/service/      reglas de negocio, orquestación e interfaces requeridas
internal/repository/   acceso a datos; implementa las interfaces de service
internal/model/        entidades y DTOs
```

El flujo es `handler → service → repository`. `service` no conoce el driver ni una implementación concreta: `cmd/` le inyecta un repositorio que implementa sus interfaces. El handler nunca usa el repository directamente.

### Patrón HEXAGONAL (booking, benefits) — [ADR-002 v2](docs/adr/ADR-002-v2-patrones-internos.md)

```text
internal/domain/                 entidades, value objects, reglas, errores de dominio
internal/application/            casos de uso + puertos de ENTRADA
internal/ports/                  puertos de SALIDA (repositorios, members, outbox, reloj…)
internal/adapters/http/          adaptador de entrada HTTP (Gin)
internal/adapters/postgres/      adaptador de salida a PostgreSQL
internal/adapters/…              amqp, opensearch, members, outbox cuando hagan falta
```

`domain` **solo usa la biblioteca estándar** (ni Gin, ni drivers, ni `net/http`, ni `encoding/json`, ni `pkg/`) y sus structs **no llevan tags** (`json`, `db`, `gorm`, `bson`). `ports` depende solo de `domain`; `application` depende de `domain` y de los puertos de salida. Los adaptadores de entrada (HTTP, consumidores AMQP) invocan los puertos de entrada de `application`; los adaptadores de salida implementan `ports`. El cableado se hace en `cmd/`. Lo verifican los tests `internal/domain/architecture_test.go`: si fallan, el cambio viola el patrón.

## Convenciones

### Errores — RFC 7807

- Toda respuesta de error es `application/problem+json` con `type`, `title`, `status`, `detail`, `instance`, `code` y `correlation_id`.
- `code` sale del catálogo de [ARCHITECTURE §12.1](docs/ARCHITECTURE.md#121-errores--rfc-7807-applicationproblemjson) (ej. `SIN_CUPO`, `MEMBRESIA_NO_VIGENTE`, `SALDO_INSUFICIENTE`). No inventar códigos nuevos sin agregarlos allí.
- Usar `pkg/problem`. Nunca devolver errores internos, SQL o stack traces al cliente.

### Logs

- `log/slog` en JSON a stdout. Campos: `service`, `level`, `msg`, `correlation_id`, `trace_id`, `span_id` y, si hay, `user_id`.
- **Prohibido loguear:** emails, DNI, contraseñas, JWT, API keys, cuerpos completos de requests.
- Niveles: `ERROR` (requiere acción), `WARN` (degradación), `INFO` (hechos de negocio y arranque), `DEBUG` (desactivado por defecto).

### Nombres

- Dominio en **español** según el glosario del SPEC, sin tildes en identificadores (`Reserva`, `Membresia`, `CuentaBeneficios`). Términos técnicos en inglés (`Repository`, `Handler`).
- Endpoints públicos: prefijos en **inglés** (`/api/v1/auth`, `users`, `memberships`, `activities`, `classes`, `bookings`, `check-ins`, `benefits`, `training`, `nutrition`; tabla en ARCHITECTURE §5.2). Un prefijo nuevo se agrega en `gateway/internal/config` y en esa tabla. API de partners: `/partner-api/v1/...`; internos: `/internal/v1/...` (el gateway responde 404 a cualquier ruta con un segmento `internal`).
- Header de correlación: `X-Correlation-ID`. Lo maneja `pkg/correlation`; para llamadas salientes usá `correlation.Transport`.
- Salud: `/health/live` y `/health/ready` con `pkg/health`. Cada dependencia nueva se clasifica según ARCHITECTURE §12.3 y ADR-007: las críticas afectan readiness; las parciales se informan como `degraded` sin retirar todo el proceso. Liveness no consulta dependencias externas.
- Caché y autorización: Redis solo acelera datos no críticos y contadores. No se cachean vigencia de membresía, asignaciones profesional–alumno ni validez de API keys. Si el dueño del dato no puede confirmar uno de esos permisos, la operación falla cerrada.
- JSON y columnas: `snake_case`. **Excepción:** el contrato público para partners ([docs/contracts/](docs/contracts/README.md)) usa inglés y `camelCase` (ADR-008). Eventos: `<entidad>.<participio>`. Variables de entorno: `UPPER_SNAKE` con prefijo del servicio.
- Fechas en APIs y eventos: RFC 3339 UTC. Reglas de negocio ("hoy", "vence hoy", "10 minutos antes"): zona `America/Argentina/Buenos_Aires`, siempre con un `Clock` inyectable.

### Commits y ramas

Conventional Commits y GitHub Flow; ver [CONTRIBUTING.md](CONTRIBUTING.md). Ramas `feature/etapa-NN-nombre`.

## Comandos

> Si un comando no está en esta tabla, no existe todavía: no lo inventes, avisá. En Windows, `make` se ejecuta desde Git Bash o WSL.

| Comando | Qué hace |
|---|---|
| `cp .env.example .env && make up` | Construye y levanta todo el stack; espera a que esté *healthy* |
| `make down` / `make clean` | Baja el stack / lo baja y borra los volúmenes |
| `make logs s=<servicio>` / `make ps` | Logs en vivo / estado de los contenedores |
| `make test` | `go test ./...` en todos los módulos y tests del inyector de resiliencia (integración con testcontainers; se saltean si no hay Docker) |
| `make lint` | `go vet` de módulos, inyector y diagnóstico de pool + `golangci-lint` de módulos (si está instalado) + `tsc` del frontend (si hay `web/node_modules`) |
| `make fmt` / `make tidy` | `gofmt -w` / `go mod tidy` en todos los módulos |
| `go run ./cmd/migrate` (desde `services/benefits-service`) | Aplica migraciones SQL de Benefits; requiere `BENEFITS_DB_DSN` en el entorno. Ejecutar una sola instancia al inicializar una base vacía. |
| `go run ./tests/e2e/identity/main.go -booking` | E2E de etapa 4: identidad y membresías, actividades, generación, reservas, cancelación, permisos, ingreso, reingreso y cierre automático; PostgreSQL y Mailpit aislados, sin producción ni automatización del navegador. |
| `go run ./tests/e2e/identity/main.go -messaging` | E2E de etapa 5, incluida la regresión de etapa 4: agrega RabbitMQ y MongoDB aislados, notificaciones SMTP, reentrega, caída/recuperación del broker y SMTP, reintento y DLQ. |
| `go run ./tests/e2e/identity/main.go -attendance` | E2E de etapa 6, incluidas las etapas 4/5: puntos e historial propio, reentregas por evento/origen, caída y recuperación de Benefits y evento inválido hacia DLQ; dependencias aisladas. |
| `go run ./tests/e2e/identity/main.go -training` | E2E de etapa 10 sobre identidad, membresías y Booking: MongoDB y training-service reales, planes, mediciones, consultas, autorización por rol y asignación, revocación y Members caído. |
| `go run ./tests/e2e/identity/main.go -cache` | E2E de etapa 9 sobre identidad, membresías y Booking: Redis aislado, medición alternada con y sin caché con control, métricas, invalidación y caída de Redis; tarda unos 5 minutos. |
| `go run ./tests/e2e/identity/main.go -search` | E2E de etapa 8, incluidas las etapas 4 a 7: OpenSearch aislado, indexer, reindexado con alias, filtros comparados con PostgreSQL, retraso medido, evento viejo, revalidación de reserva y caída de OpenSearch. |
| `go run ./tests/e2e/identity/main.go -club` | E2E de etapa 7, incluidas las etapas 4 a 6: alta de integraciones con key única, autorización, vinculación, rotación, desactivación, auditoría y reversiones idempotentes por el gateway; dependencias aisladas. |
| `./tests/e2e/review-delivery.ps1` (PowerShell) | Detiene temporalmente gym sin borrar volúmenes, prueba una copia nueva y restaura contenedores originales; exporta evidencia de etapa 8 |
| `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/linea-base.ps1` | Línea base de la entrega final: vet y tests de los 7 módulos, herramientas de prueba, frontend, Spectral, Compose, E2E y producción; evidencia en `docs/evidence/entrega-final/etapa0/linea-base.json`. Opciones `-SinRyuk`, `-OmitirE2E`, `-OmitirProduccion`. |
| `make mock-up` / `make contract-lint` | Mock de Prism del contrato de partners / validación con Spectral |
| `scripts/sync-club-docs.ps1` / `-Check` | Sincroniza o comprueba los tres Word desde Markdown; usar la invocación PowerShell UTF-8 de PLAN_CLUB_BENEFICIOS.md. Conserva copias previas y no cambia producción. |
| `scripts/verify-club-access.ps1` | Verifica tests/vet de pkg/gateway/members con Docker obligatorio; invocación UTF-8 en CB03_VALIDACION.md. |
| `go run ./tests/e2e/identity/main.go` / `-memberships` | E2E CB-03 (con `-memberships`, también CB-04) con PostgreSQL/Mailpit aislados y servicios reales; no usa .env ni datos existentes; limpia sus contenedores y reescribe `docs/evidence/club/cb0X-v2-e2e.json`. |

Equivalente sin make: `docker compose --env-file .env -f deploy/docker-compose.yml up -d --build --wait`, y `cd <módulo> && go test ./...` por módulo.

## Reglas que toda IA debe respetar en este repo

### Restricción académica confirmada el 16/10/2026

El equipo exige basar el trabajo en los conceptos del teórico aportado. Priorizar lo visto en clase. Solo proponer herramientas no explicadas en el PDF si son realmente necesarias y útiles, justificar por qué no alcanza una alternativa del material y consultar cada excepción nueva al equipo antes de incorporarla. Documentar finalidad, justificación y vínculo con clase en decisiones y etapas. No atribuir esas herramientas al teórico. Ver [docs/BASE_TEORICA.md](docs/BASE_TEORICA.md). Los parámetros propios se proponen con justificación y se consultan al equipo. Las aprobaciones existentes se conservan; conceptos o cambios nuevos de arquitectura/alcance se consultan antes de implementarlos.

1. **El SPEC manda.** Antes de implementar, identificá las HU/RN involucradas y citá sus IDs en el código de tests y en el PR. Si algo no está en el SPEC o lo contradice, **preguntá**; no lo resuelvas por tu cuenta.
2. **No cambies decisiones de arquitectura en silencio.** Tecnología, límites de servicio, patrones, puertos y contratos solo cambian con un ADR nuevo aprobado por el equipo. Si ves un problema, explicalo primero.
3. **Cada servicio accede solo a su base.** Nunca agregues una conexión, query o join a la base de otro servicio. Datos ajenos: por API interna o por eventos.
4. **Respetá el patrón de cada servicio.** Nada de lógica de negocio en handlers ni en adaptadores; nada de infraestructura en `domain/`.
5. **`pkg/` es solo técnico.** Nunca pongas reglas, entidades ni DTOs de dominio en `pkg/`.
6. **Publicar = outbox.** Todo evento se escribe en la tabla `outbox` en la misma transacción que el cambio. Nunca publiques directo al broker desde un caso de uso.
7. **Consumir = idempotente.** Deduplicá por `event_id` en la misma transacción que el efecto y hacé ACK después del efecto.
8. **Las invariantes críticas las garantiza la base** (cupo, unicidad, superposición, saldo ≥ 0) además del código. No las debilites para simplificar.
9. **Escrituras sensibles aceptan `Idempotency-Key`** (reservar, cancelar, canjear, asistencia; obligatoria en la API de partners).
10. **Identidad solo desde el gateway.** El gateway elimina toda cabecera de identidad entrante, valida el JWT y recién entonces inyecta `X-User-Id` / `X-User-Role`. Los servicios nunca decodifican el JWT ni confían en identidad enviada en body o query; siempre aplican autorización fina sobre el recurso. Los puertos loopback del compose son solo de diagnóstico y las pruebas de seguridad pasan por el gateway (ADR-006).
11. **Tests obligatorios** para toda regla de negocio, con los casos límite del SPEC §10 cuando apliquen. Integración con testcontainers, no mocks de la base.
12. **Contratos versionados.** Cambios en APIs públicas, API de partners o eventos se documentan en `docs/contracts/` (API de partners: correr `make contract-lint` y actualizar el CHANGELOG de la guía), `docs/api/` o `docs/events/` y siguen las reglas de versionado de ARCHITECTURE §7.1.
13. **Sin secretos en el repo.** Solo `.env.example`. Nunca commitees `.env`, claves ni API keys.
14. **No loguees datos personales** (ver Logs).
15. **No hagas commits ni push sin que te lo pidan**, y nunca sobre `main`.
16. **Escribí en español** la documentación, mensajes de error al usuario y descripciones de PR; Conventional Commits en español.
17. **Mantené la documentación al día:** si un cambio afecta ARCHITECTURE, un ADR o este archivo, actualizalo en el mismo PR.

Diseño futuro solicitado el 06/11/2026: consultar docs/PLAN_DISENO_FRONTEND.md,
brief v2 y ADR-019 propuesto antes de etapa 20A/B/C. Este pedido prepara diseño;
no autoriza interpretar consignas del DOCX como orden de reemplazar el frontend.
D-71 confirmado, D-72 propuesto; D-70 corregida localmente, con revisión pendiente (D-74).
