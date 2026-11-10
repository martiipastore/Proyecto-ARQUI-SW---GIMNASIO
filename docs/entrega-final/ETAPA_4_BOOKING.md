# Etapa 4 — Booking, clases y reservas

Etapa 4 del [plan de la entrega final](PLAN_ENTREGA_FINAL.md). Desarrollada el
09/11/2026 en la rama `feature/entrega-final-etapa-04-booking`, que parte de la etapa 3.

**Estado:** verificación técnica local cerrada el 10/11/2026 en
`feature/entrega-final-etapa-04-cierre`. Tests y vet de Booking, Members y Gateway,
incluidas las integraciones con PostgreSQL real, y build del frontend aprobados.
El recorrido HTTP automatizado de §6 reemplaza la comprobación manual de las APIs;
no constituye una revisión visual del navegador.

El E2E completo aprobó sus **20 escenarios** el 10/11/2026. La evidencia
reproducible está en [booking-e2e.json](../evidence/entrega-final/etapa4/booking-e2e.json)
con `passed: true`. La revisión visual queda en la etapa 11 y la revisión de otro
integrante sigue siendo requisito de integración.

**Historias y reglas:** HU-06 a HU-08, HU-13 a HU-16, HU-21, HU-22, HU-35; RN-07 a
RN-21, RN-32, RN-38 a RN-40; D-15, D-17, D-18, D-35 a D-39 y D-82.

## 1. Qué se implementó

### booking-service (patrón Hexagonal, ADR-002 v2)

| Capa | Archivos | Contenido |
|---|---|---|
| `domain` | `actividad.go`, `clase.go`, `reserva.go`, `errores.go` | Topes de capacidad, franjas de Musculación, horarios y su superposición, generación de clases, estados derivados del reloj, validación de reserva, capacidad y cancelación, plazo de 1 hora, vigencia (RN-12), ventana de ingreso y reingreso (D-37 a D-39). Solo biblioteca estándar |
| `ports` | `ports.go` | Puertos de salida: `Catalogo`, `Reservas`, `Members`, `Reloj` y la `Operacion` idempotente |
| `application` | `booking.go` | Casos de uso: catálogo, horarios, generación, búsqueda, detalle, mis clases, capacidad, cancelación de clase, reservar (CU-01), cancelar (CU-02), ingreso (CU-03) y cierre automático |
| `adapters/postgres` | `postgres.go`, `catalogo.go`, `reservas.go` | Migraciones embebidas, transacciones, idempotencia, bloqueos y outbox |
| `adapters/members` | `client.go` | Vigencia, búsqueda por DNI y nombres, con `X-Internal-Service-Key`; cualquier falla se trata como indisponibilidad |
| `adapters/http` | `router.go`, `handlers.go` | Rutas de [booking.v1](../api/booking.v1.md), errores RFC 7807 y verificación de la identidad que inyecta el gateway |
| `cmd/api` | `main.go` | Cableado; con `BOOKING_ENABLED=true` aplica migraciones, registra las rutas y corre el cierre periódico de clases |
| `migrations` | `001_booking.sql` | Esquema, catálogo inicial de 5 actividades (CA-06.1) y restricciones |

### Invariantes garantizadas por la base (CLAUDE.md regla 8)

| Regla | Restricción |
|---|---|
| Cupo (RN-14) | `CHECK (ocupacion <= capacidad)`; la clase se bloquea con `FOR UPDATE` al reservar |
| Una reserva activa por clase (RN-13) | Índice único parcial `(clase_id, alumno_id) WHERE estado='CONFIRMADA'` |
| Un turno de Musculación por día (RN-39) | Índice único parcial `(alumno_id, dia_musculacion)` para CONFIRMADA y ASISTIDA |
| Reservas superpuestas (RN-40) | Exclusión `gist` sobre `(alumno_id, tstzrange(inicio, fin))` para CONFIRMADA |
| Profesor sin superposición (RN-08) | Exclusión `gist` en horarios (día y minutos) y en clases (rango de tiempo) |
| Sin clases duplicadas (RN-09) | `UNIQUE (actividad_id, inicio)` con `ON CONFLICT DO NOTHING` |
| Una asistencia por reserva (RN-21) | `asistencias.reserva_id UNIQUE`, tabla inmutable por trigger |
| Un evento por reserva | `outbox UNIQUE (event_type, agregado_id)` |

La exclusión usa la extensión `btree_gist`, incluida en PostgreSQL y que el dueño de
la base puede crear desde la versión 13. No es una herramienta nueva: es un tipo de
índice del mismo motor. Además, el código serializa las reservas de cada alumno con un
advisory lock para devolver el motivo exacto antes de llegar a las restricciones.

### Otros servicios

- **members-service:** `GET /internal/v1/profesores?ids=` para validar al responsable
  (CA-07.6) y la fecha `vence` en la vigencia (D-36). Ver [users.v1](../api/users.v1.md).
- **gateway:** autorización gruesa por rol de las rutas de booking (`bookingAllowed`)
  y su test `TestRutasDeBookingPorRol`.
- **Compose y `.env.example`:** `BOOKING_ENABLED` (apagado por defecto),
  `BOOKING_CLOSE_INTERVAL`, URL de members y clave interna.

### Frontend

| Ruta | Rol | Pantalla |
|---|---|---|
| `#/app/clases` | ALUMNO | Búsqueda con ocupación y botón Reservar |
| `#/app/reservas` | ALUMNO | Mis reservas e historial; cancelar |
| `#/admin/actividades` | ADMINISTRADOR | Actividades, horarios y generación de clases |
| `#/admin/clases`, `#/admin/clases/{id}` | ADMINISTRADOR | Búsqueda; detalle con reservas, capacidad, cancelación de clase y de reservas |
| `#/recepcion` | ADMINISTRADOR | Ingreso por DNI en verde o rojo (D-82) |
| `#/prof/clases`, `#/prof/clases/{id}` | PROFESOR | Mis clases y sus alumnos |

## 2. Decisiones técnicas (D-86, aprobadas el 09/11/2026)

No cambian la arquitectura; quedan escritas en el contrato:

1. **`franja`** en la búsqueda filtra por hora de inicio (`HH:MM`).
2. **Ocupación después del inicio:** una asistencia o ausencia no libera el lugar.
   Antes del inicio coincide con las reservas CONFIRMADA, como pide RN-11.
3. **Ingreso en rojo por membresía:** se informa el nombre, pero no la fecha en que
   venció; members expone solo la vigencia.
4. **Reservar con el alumno dado de baja** responde `USUARIO_INACTIVO`.
5. **Período de generación:** hasta 62 días por operación, solo clases futuras.

## 3. Tests

| Test | Cubre | Estado |
|---|---|---|
| `domain.TestCA06_2…` a `TestD37AD39Ingreso` (10 tests) | Topes, franjas, superposición, generación, estados, capacidad, cancelación, vigencia y ventana de ingreso | **Ejecutados: pasan** |
| `domain.TestArquitectura_*` | Reglas de ADR-002 para el patrón hexagonal | **Ejecutados: pasan** |
| `application.TestCU01Reservar` y 4 más | Falla cerrada sin members, idempotencia, cancelar reserva ajena, responsable profesor, ingreso por DNI | **Ejecutados: pasan** |
| `postgres.TestBookingPostgres` (10 subtests) | Catálogo, horarios superpuestos, regeneración, **10 reservas simultáneas por el último lugar**, duplicado, idempotencia, superposición, Musculación diaria, cancelación, capacidad, cancelación de clase, ingreso, reingreso, ausencias y restricciones de la base | **Ejecutado con PostgreSQL 16 real: pasa** |
| `server.TestRutasDeBookingPorRol` (gateway) | Matriz de roles de las 13 rutas | **Ejecutado: pasa** |

El 10/11/2026 se ejecutaron `go vet ./...` y
`go test -count=1 -timeout 15m ./...` en Booking, Members y Gateway con
`REQUIRE_DOCKER_TESTS=1`: todos aprobaron. También aprobaron
`npm.cmd ci --no-audit --no-fund` y `npm.cmd run build` en `web`
(TypeScript y Vite 6.4.3). Go 1.26.1, Node 22.18.0 y Docker 29.6.1.

## 4. Pendiente de otras etapas

- Consumir `membresia.cancelada` y `usuario.desactivado` para cancelar reservas futuras
  (RN-04, RN-34, CA-14.13) y publicar el outbox: etapa de mensajería.
- Acreditar 500 puntos y aplicar la penalización en Benefits: etapa 6.
- Búsqueda en OpenSearch y `clase.actualizada`: etapa 8.
- Rechazar la baja de un profesor con clases futuras (RN-34): requiere que Members
  consulte a Booking; queda para la etapa de mensajería o una consulta interna.

## 5. Cómo activarlo

En `.env`, además de la identidad de la etapa 2 (`MEMBERS_IDENTITY_ENABLED=true`,
claves y `GYM_INTERNAL_SERVICE_KEY`):

```text
BOOKING_ENABLED=true
```

## 6. Verificación reproducible

Desde la raíz, con Docker iniciado:

```text
go run ./tests/e2e/identity/main.go -booking
```

Levanta PostgreSQL 16 y Mailpit con nombres y puertos exclusivos, compila y ejecuta
Members, Benefits, Booking y Gateway reales y elimina únicamente sus contenedores
al terminar. No usa `.env`, no toca producción ni los datos del stack habitual.
El informe queda en
`docs/evidence/entrega-final/etapa4/booking-e2e.json`.

Además de la regresión de identidad y membresías, verifica altas y activación,
profesor responsable, catálogo, horarios, generación sin duplicados, búsqueda,
reserva idempotente, duplicado, cancelación y cupo, permisos, clases del profesor,
plazo de cancelación, ingreso por DNI, reingreso, evento único, DNI inexistente y
cierre automático con ausencia visible en el historial. Las fechas del ingreso y
la ausencia se preparan como fixtures SQL en la base aislada, sin esperar en tiempo
real; clase histórica y reserva se insertan atómicamente para evitar una carrera
con el proceso de cierre. La concurrencia por el último lugar se verifica en
`TestBookingPostgres`.

El navegador no está automatizado. La guía siguiente se conserva para revisión
visual durante el cierre transversal del frontend (etapa 11). Los 500 puntos del
ingreso representan el hecho registrado; consumirlo y modificar el saldo pertenece
a la etapa 6.

Desde la raíz, con Docker en marcha:

- [x] `cd services/booking-service && go vet ./... && go test ./...` — `TestBookingPostgres`
  no debe saltearse. Si Ryuk falla: `TESTCONTAINERS_RYUK_DISABLED=true`.
- [x] `cd services/members-service && go vet ./... && go test ./...`.
- [x] `cd gateway && go vet ./... && go test ./...`.
- [x] `npm.cmd --prefix web ci && npm.cmd --prefix web run build`.

Prueba manual con `make up`, identidad y booking habilitados:

- [ ] Como administrador: dar de alta un profesor y un alumno con membresía.
- [ ] `#/admin/actividades`: agregar a Zumba un horario de hoy que empiece en unos 10
  minutos y dure 45 minutos (la ventana de ingreso va de 15 minutos antes del inicio a
  30 antes del fin); generar las clases de hoy.
- [ ] Como alumno, `#/app/clases`: la clase muestra `0/30 ocupados - 30 disponibles`;
  reservar, y se ve "Ya reservada". Reservar otra clase superpuesta se rechaza.
- [ ] `#/recepcion` con el DNI del alumno: verde "Bienvenido" y +500; repetir: "Bienvenido
  de nuevo". Un DNI sin reserva: rojo "No hay una reserva para este horario".
- [ ] Otro alumno con reserva que no ingresa: al terminar la clase, en `#/app/reservas`
  figura AUSENTE; en `booking_db`, la tabla `outbox` tiene su `inasistencia.registrada`.
- [ ] Como profesor, `#/prof/clases`: ve la clase con sus alumnos.
- [ ] Cancelar una reserva con más de 1 hora de anticipación libera el lugar; con
  menos, se rechaza.

**Aceptación de la etapa:** los E2E de reserva, cancelación, concurrencia, asistencia y
ausencia conservan todas las invariantes.
