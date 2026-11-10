# Etapa 4 — Booking, clases y reservas

Etapa 4 del [plan de la entrega final](PLAN_ENTREGA_FINAL.md). Desarrollada el
09/11/2026 en la rama `feature/entrega-final-etapa-04-booking`, que parte de la etapa 3.

**Estado:** el código, los tests y la documentación están completos. Los tests de
dominio y de casos de uso se ejecutaron y pasan. Falta correr la integración con
PostgreSQL (Docker), el build del frontend y la prueba manual (§6).

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
| `postgres.TestBookingPostgres` (10 subtests) | Catálogo, horarios superpuestos, regeneración, **10 reservas simultáneas por el último lugar**, duplicado, idempotencia, superposición, Musculación diaria, cancelación, capacidad, cancelación de clase, ingreso, reingreso, ausencias y restricciones de la base | Escrito; requiere Docker |
| `server.TestRutasDeBookingPorRol` (gateway) | Matriz de roles de las 13 rutas | Escrito; no ejecutado |

`go vet` y `gofmt` pasan en dominio, puertos y casos de uso. El resto del código se
revisó con el analizador de Go del editor, sin errores, pero no se compiló: el equipo
de desarrollo no tiene espacio para compilar las dependencias.

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

## 6. Verificación pendiente

Desde la raíz, con Docker en marcha:

- [ ] `cd services/booking-service && go vet ./... && go test ./...` — `TestBookingPostgres`
  no debe saltearse. Si Ryuk falla: `TESTCONTAINERS_RYUK_DISABLED=true`.
- [ ] `cd services/members-service && go vet ./... && go test ./...`.
- [ ] `cd gateway && go vet ./... && go test ./...`.
- [ ] `npm --prefix web ci && npm --prefix web run build`.

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
