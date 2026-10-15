# ADR-002 — Patrón interno de cada servicio (Capas y Hexagonal)

| Campo | Valor |
|---|---|
| **Estado** | Reemplazado por [ADR-002 v2](ADR-002-v2-patrones-internos.md) |
| **Fecha** | 2026-10-07 |
| **Decisión del TP** | D2 |
| **Autores** | Pastore, Martina · Schaffer, Matías · Viti, María Guadalupe |
| **Relacionado con** | [ADR-001](ADR-001-limites-de-servicios.md); [ARCHITECTURE §4.2 y §13](../ARCHITECTURE.md); [CLAUDE.md](../../CLAUDE.md) |

## Contexto

ADR-001 definió cinco servicios con complejidades muy distintas:

- **members-service** y **training-service** son mayormente altas, bajas y consultas con validaciones acotadas.
- **booking-service** concentra las reglas más delicadas del SPEC (cupo con concurrencia, unicidad, superposición, ingreso mediante DNI y ausencias automáticas; RN-12 a RN-21, RN-38 a RN-40) y además tiene un modelo de lectura (CQRS) y dos procesos (api e indexer).
- **benefits-service** tiene un ledger con invariantes estrictas (RN-22 a RN-27) y publica un contrato a terceros (ADR-008) que debe mantenerse estable aunque cambie la infraestructura.

Usar el mismo patrón para todos haría que los servicios simples carguen indirecciones inútiles, o que los complejos mezclen reglas con infraestructura. Además, el TP pide mostrar y justificar más de un estilo de arquitectura interna.

## Alternativas consideradas

### Alternativa A — Capas en todos los servicios

| Pros | Contras |
|---|---|
| Simple y conocida por todo el equipo | En booking y benefits las reglas terminan acopladas a Gin y a pgx; probarlas exige base de datos |
| Menos archivos | Cambiar un adaptador (ej. OpenSearch) toca la lógica de negocio |

### Alternativa B — Hexagonal en todos los servicios

| Pros | Contras |
|---|---|
| Un solo estilo | Puertos y adaptadores para CRUD simples: mucho código sin beneficio |
| Dominio aislado en todos | Más tiempo de desarrollo en servicios que no lo necesitan |

### Alternativa C — Patrón según complejidad (elegida)

Capas donde predomina el CRUD; Hexagonal donde hay reglas críticas o un contrato externo.

| Pros | Contras |
|---|---|
| Cada servicio paga solo la complejidad que necesita | Dos estilos que aprender y revisar |
| Las reglas críticas se prueban sin infraestructura | Hay que vigilar que no se mezclen convenciones |

## Decisión

| Servicio | Patrón |
|---|---|
| `members-service` | **Capas** |
| `training-service` | **Capas** |
| `booking-service` (+ `booking-indexer`) | **Hexagonal** + CQRS de lectura |
| `benefits-service` | **Hexagonal** |
| `notification-worker` | Consumidor simple (`consumer` → `mailer` / `store`) |
| `api-gateway` | Pipeline de middlewares + proxy |

### Capas

```text
cmd/api/main.go          cableado y arranque
internal/config/         configuración desde el entorno
internal/handler/        HTTP (Gin): rutas, formato de entrada, errores RFC 7807
internal/service/        reglas de negocio; depende de interfaces de repository
internal/repository/     acceso a datos (único lugar que conoce el driver)
internal/model/          entidades y DTOs
```

Dependencias solo hacia abajo: `handler → service → repository`. El handler nunca usa el repository.

### Hexagonal

```text
cmd/api/main.go (y cmd/indexer/)   cableado: crea adaptadores y se los pasa a los casos de uso
internal/config/                   configuración desde el entorno
internal/domain/                   entidades, value objects, reglas y errores de dominio
internal/application/              casos de uso + puertos de ENTRADA
internal/ports/                    puertos de SALIDA (repositorios, members, outbox, reloj…)
internal/adapters/http/            adaptador de entrada HTTP (Gin)
internal/adapters/postgres/        adaptador de salida a PostgreSQL
internal/adapters/…                otros adaptadores (amqp, opensearch, members) a medida que se necesiten
```

Reglas:

1. `domain` solo usa la biblioteca estándar: no importa frameworks, drivers ni `encoding/json`, y sus structs **no llevan tags** (`json`, `db`, `gorm`, `bson`).
2. `application` y `ports` pueden usar `domain`, pero nunca `adapters`, Gin, pgx, Mongo, `net/http` ni `database/sql`.
3. Los adaptadores implementan los puertos y traducen formatos (JSON, filas, documentos) desde y hacia el dominio.
4. Solo `cmd/` conoce todas las piezas.

### Código compartido

`pkg/` (módulo propio) contiene solo piezas técnicas: configuración desde el entorno, logger slog JSON con `correlation_id`, middleware de correlation ID, errores RFC 7807, endpoints de salud, router Gin base y servidor con graceful shutdown. **Nunca** reglas, entidades ni DTOs de dominio.

## Consecuencias

### Positivas

- Las reglas de cupo y de saldo se prueban con tests unitarios puros, sin Docker.
- El contrato de partners (ADR-008) se implementa como un adaptador más de benefits; cambios de infraestructura no lo afectan.
- members y training se desarrollan rápido, sin ceremonia.

### Negativas

- Dos estructuras de carpetas distintas en el repo.
- En hexagonal hay mapeos explícitos entre dominio y adaptadores (más código).

### Riesgos aceptados

| Riesgo | Probabilidad | Impacto | Mitigación |
|---|---|---|---|
| Se filtra infraestructura al dominio por comodidad | Media | Alto | Tests de arquitectura en booking y benefits (abajo) que fallan la CI |
| Lógica de negocio en handlers de los servicios en capas | Media | Medio | Revisión de PR (checklist de CONTRIBUTING) |
| `pkg/` crece con lógica de dominio | Baja | Medio | Regla en CLAUDE.md y revisión de PR |

## Cómo se verificará

- **Tests de arquitectura** (`internal/domain/architecture_test.go` en booking-service y benefits-service), que corren con `go test` y en la CI:
  - `domain` no importa nada fuera de la biblioteca estándar ni `net/http`, `database/sql` o `encoding/json`;
  - ninguna struct de `domain` tiene tags;
  - `application` y `ports` no importan adaptadores ni frameworks.
- **Módulos Go separados** por servicio (`go.work`): un servicio no puede importar `internal/` de otro.
- **Revisión de PR:** el checklist de CONTRIBUTING pide confirmar que el cambio respeta el patrón del servicio.
