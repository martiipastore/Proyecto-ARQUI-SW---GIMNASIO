# CLAUDE.md

Contexto y reglas para asistentes de IA (Claude u otros) que trabajen en este repositorio. Leelo completo antes de proponer o escribir cambios.

## Proyecto

Sistema integral de gestión de gimnasio: Trabajo Práctico Integrador de Arquitectura de Software, construido como **microservicios**. Incluye membresías, clases y reservas con cupo, asistencia, planes de entrenamiento y nutrición, y un Club de Beneficios con puntos expuesto a partners de otros grupos.

- **Qué hace el sistema:** [SPEC.md](SPEC.md) (aprobado; fuente de verdad del negocio).
- **Cómo está construido:** [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) y [docs/adr/](docs/adr/).
- **Etapa actual:** esqueleto del proyecto — monorepo Go con `go.work`, `pkg/` compartido, los 5 servicios + indexer + gateway con `/health/live` y `/health/ready`, frontend con la pantalla "Estado del sistema", Docker Compose y CI. **Todavía no hay funcionalidades de negocio** (usuarios, membresías, clases, reservas, entrenamiento, nutrición ni beneficios).

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
| `members-service` | 8081 | **Capas** | PostgreSQL `members_db` | `membresia.activada`, `membresia.cancelada`, `usuario.desactivado` | — |
| `booking-service` (×2 con Traefik desde la etapa de balanceo; hoy ×1) | 8082 | **Hexagonal + CQRS** | PostgreSQL `booking_db` + OpenSearch | `asistencia.registrada`, `inasistencia.registrada`, `clase.actualizada` | `membresia.cancelada`, `usuario.desactivado` |
| `booking-indexer` | 8086 (solo health) | Consumidor | OpenSearch `clases` | — | `clase.actualizada` |
| `benefits-service` | 8083 | **Hexagonal** | PostgreSQL `benefits_db` | — | `asistencia.registrada`, `inasistencia.registrada` |
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
| `make test` | `go test ./...` en todos los módulos (integración con testcontainers; se saltean si no hay Docker) |
| `make lint` | `go vet` + `golangci-lint` (si está instalado) + `tsc` del frontend (si hay `web/node_modules`) |
| `make fmt` / `make tidy` | `gofmt -w` / `go mod tidy` en todos los módulos |
| `make mock-up` / `make contract-lint` | Mock de Prism del contrato de partners / validación con Spectral |

Equivalente sin make: `docker compose --env-file .env -f deploy/docker-compose.yml up -d --build --wait`, y `cd <módulo> && go test ./...` por módulo.

## Reglas que toda IA debe respetar en este repo

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
