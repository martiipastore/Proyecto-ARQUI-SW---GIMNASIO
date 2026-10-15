# ADR-002 v2 — Patrón interno de cada servicio (Capas y Hexagonal)

| Campo | Valor |
|---|---|
| **Estado** | Aceptado |
| **Fecha** | 2026-10-08 |
| **Decisión del TP** | D2 |
| **Reemplaza** | [ADR-002](ADR-002-patrones-internos.md) |
| **Autores** | Pastore, Martina · Schaffer, Matías · Viti, María Guadalupe |
| **Relacionado con** | [ADR-001](ADR-001-limites-de-servicios.md); [ARCHITECTURE §4.2 y §13](../ARCHITECTURE.md); [CLAUDE.md](../../CLAUDE.md) |

## Motivo de la revisión

La decisión original seleccionó correctamente dos patrones internos, pero describió como consecuencia necesaria de la arquitectura en capas que las reglas quedaran acopladas a Gin y pgx y que sus pruebas exigieran una base de datos. Ese acoplamiento depende de cómo se implementen las capas; no es una propiedad del patrón.

Esta revisión conserva la asignación de patrones y precisa sus dependencias, puertos y adaptadores. No cambia los límites de servicios ni las tecnologías elegidas.

## Contexto

Los servicios tienen complejidades diferentes:

- **members-service** y **training-service** concentran operaciones de alta, modificación y consulta con reglas acotadas.
- **booking-service** contiene reglas críticas de cupo, concurrencia, superposición, ingreso y ausencias, además de un modelo de lectura y adaptadores externos.
- **benefits-service** protege las invariantes del ledger y mantiene un contrato público estable para terceros.

El TP también requiere aplicar y justificar al menos dos estilos de arquitectura interna.

## Alternativas consideradas

### Alternativa A — Capas en todos los servicios

| Ventajas | Desventajas |
|---|---|
| Estructura conocida y directa para el equipo | En booking y benefits, las fronteras entre casos de uso e infraestructura quedarían menos explícitas |
| Menor cantidad de abstracciones para flujos simples | Exige disciplina adicional para que handlers y repositorios no absorban reglas de negocio |
| Puede usar interfaces e inyección de dependencias para aislar infraestructura | La incorporación de varios mecanismos de entrada y salida vuelve menos clara la estructura por capas técnicas |

### Alternativa B — Hexagonal en todos los servicios

| Ventajas | Desventajas |
|---|---|
| Fronteras explícitas alrededor de la aplicación y el dominio | Agrega interfaces, adaptadores y mapeos incluso en operaciones simples |
| Facilita sustituir infraestructura y probar casos de uso aislados | Aumenta el costo de desarrollo y revisión en servicios con reglas acotadas |

### Alternativa C — Patrón según complejidad (elegida)

Capas para los servicios con flujos predominantemente simples; Hexagonal para los que concentran invariantes críticas, varios adaptadores o un contrato externo.

| Ventajas | Desventajas |
|---|---|
| Ajusta la complejidad estructural a las necesidades de cada servicio | El equipo debe mantener y revisar dos estructuras diferentes |
| Hace explícitas las fronteras de booking y benefits | Requiere convenciones claras para no mezclar ambos estilos |

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
internal/handler/        entrada HTTP: parseo, validación de formato y respuestas
internal/service/        casos de uso, reglas de negocio e interfaces requeridas
internal/repository/     acceso a datos e implementación de las interfaces
internal/model/          entidades y DTOs internos
```

El flujo de una solicitud es `handler → service → repository`, pero la dependencia de la lógica hacia la persistencia se expresa mediante interfaces: `service` no conoce el driver ni una implementación concreta. `cmd/` construye el repositorio y lo inyecta en el servicio. El handler tampoco accede directamente al repositorio.

Este patrón permite probar las reglas del servicio con dobles de sus interfaces. Las pruebas con una base real se reservan para el repositorio, las restricciones y la integración.

### Hexagonal

```text
cmd/api/main.go (y cmd/indexer/)   cableado de la aplicación y sus adaptadores
internal/config/                   configuración desde el entorno
internal/domain/                   entidades, value objects, reglas y errores de dominio
internal/application/              casos de uso y puertos de entrada
internal/ports/                    puertos de salida requeridos por la aplicación
internal/adapters/http/            adaptador de entrada HTTP (Gin)
internal/adapters/amqp/            adaptador de entrada/salida AMQP, según el flujo
internal/adapters/postgres/        adaptador de salida a PostgreSQL
internal/adapters/…                otros adaptadores de salida (OpenSearch, members, reloj)
```

- Un **puerto de entrada** es una interfaz o contrato de caso de uso que la aplicación ofrece, por ejemplo `ReservarClase` o `AcreditarPuntos`.
- Un **adaptador de entrada** recibe una interacción externa y llama a un puerto de entrada, por ejemplo un handler Gin o un consumidor AMQP.
- Un **puerto de salida** expresa una necesidad de la aplicación, por ejemplo `ReservaRepository`, `EventPublisher` o `MembresiaChecker`.
- Un **adaptador de salida** implementa un puerto de salida mediante una tecnología concreta, por ejemplo PostgreSQL, RabbitMQ, OpenSearch o un cliente HTTP.

Reglas de dependencia:

1. `domain` solo usa la biblioteca estándar: no importa frameworks, drivers ni `encoding/json`, y sus structs no llevan tags de transporte o persistencia.
2. `application` depende de `domain` y de abstracciones de salida, nunca de adaptadores concretos.
3. Los adaptadores dependen de los puertos que invocan o implementan y traducen sus formatos hacia o desde el dominio.
4. Solo `cmd/` conoce las implementaciones concretas necesarias para realizar el cableado.

### Código compartido

`pkg/` contiene únicamente piezas técnicas reutilizables: configuración, logging, correlación, errores HTTP, salud, router base y cierre ordenado. No contiene reglas, entidades ni DTOs de dominio.

## Consecuencias

### Positivas

- Las reglas de booking y benefits se prueban sin depender de sus adaptadores.
- Members y training conservan una estructura directa y también pueden aislar la persistencia mediante interfaces.
- Los contratos HTTP, AMQP y de almacenamiento quedan ubicados en fronteras explícitas.
- La terminología usada en el proyecto coincide con la arquitectura hexagonal vista en la materia.

### Negativas

- Existen dos estructuras internas que el equipo debe conocer.
- Los servicios hexagonales requieren mapeos e interfaces adicionales.
- La arquitectura en capas depende de revisiones que eviten lógica en handlers o repositorios.

## Cómo se verificará

- Tests unitarios de los casos de uso usando implementaciones controladas de sus interfaces.
- Tests de integración para repositorios, restricciones y adaptadores externos.
- Tests de arquitectura en booking y benefits que impidan dependencias desde `domain` o `application` hacia infraestructura.
- Revisión de PR conforme al patrón asignado a cada servicio y al checklist de `CONTRIBUTING.md`.
