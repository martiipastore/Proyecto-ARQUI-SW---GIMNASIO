# Etapa 3 — Members de punta a punta

Etapa 3 del [plan de la entrega final](PLAN_ENTREGA_FINAL.md). Desarrollada el
09/11/2026 en la rama `feature/entrega-final-etapa-03-members`.

**Estado:** el código y la documentación están completos. Falta compilar, correr los
tests con Docker y construir el frontend con Node (§5): el equipo donde se desarrolló
se quedó sin espacio en disco y no tiene Node.

**Historias y reglas:** HU-01, HU-02, HU-03, HU-21, RN-30, RN-33, RN-34, RN-36,
CA-01.1 a CA-01.4, CA-02.1 a CA-02.4, SPEC §9 y D-77/D-78.

## 1. Qué se implementó

### Backend (members-service, patrón Capas)

| Archivo | Contenido |
|---|---|
| `migrations/004_padron_personas.sql` | `usuarios` suma apellido, DNI único de 7 u 8 dígitos, teléfono, fecha de nacimiento (≥ 1900), matrícula, estado `ACTIVO`/`INACTIVO` y datos de la baja. Restricciones en la base: matrícula solo para nutricionistas, padrón completo para alumnos y profesionales, baja coherente. Tablas nuevas `asignaciones` (índice único parcial sobre la vigente), `activaciones_cuenta`, `outbox_usuarios` (`alumno.creado`, `usuario.desactivado`) y `auditoria_usuarios` (inmutable por trigger) |
| `internal/model/people.go` | `Person`, entradas, filtros, página, `Assignment`, `StudentSummary` y errores de duplicado |
| `internal/model/membership.go` | `CoveringMembership` (qué membresía cubre una fecha) y la respuesta de vigencia |
| `internal/service/people.go` | Reglas: alta, modificación, baja, asignar y revocar, mis alumnos, búsquedas internas y administrador inicial |
| `internal/service/membership.go` | `Coverage`: vigencia hoy y en hasta 31 fechas; un alumno dado de baja no tiene vigencia |
| `internal/service/identity.go` | El login rechaza personas `INACTIVO` (CA-01.4); el worker envía el correo "Activá tu cuenta del gimnasio" |
| `internal/repository/people.go` | Persistencia con transacción: locks por email y DNI, completar al usuario externo, fila de activación, outbox y auditoría en la misma transacción |
| `internal/handler/people.go` | Rutas públicas y internas, errores RFC 7807 |
| `internal/config/config.go`, `cmd/api/main.go` | Variables `MEMBERS_ADMIN_*` y cableado |

Contrato: [users.v1.md](../api/users.v1.md).

| Ruta | Quién |
|---|---|
| `GET/POST /api/v1/users`, `GET/PUT /api/v1/users/{id}` | Administrador (el detalle también la propia persona) |
| `POST /api/v1/users/{id}/deactivate` | Administrador |
| `POST /api/v1/users/{id}/assignments`, `.../{alumnoId}/revoke` | Administrador |
| `GET /api/v1/users/me/students` | Profesor o nutricionista |
| `GET /internal/v1/alumnos?ids=`, `POST /internal/v1/alumnos/buscar-dni` | Booking (etapa 4) |
| `GET /internal/v1/asignaciones` | Training (etapa 10) |
| `GET /internal/v1/alumnos/{id}/vigencia?fechas=` | Booking, para reservar (RN-12) |

Las rutas internas exigen `X-Internal-Service-Key` y el gateway les responde 404.

### Gateway

`gateway/internal/proxy/access.go` agrega la regla gruesa de rol para
`GET /api/v1/users/me/students` (solo `PROFESOR` y `NUTRICIONISTA`). Members vuelve a
autorizar sobre el recurso.

### Frontend

| Ruta | Pantalla |
|---|---|
| `#/admin/personas` (`#/admin` redirige aquí) | Listado con filtros por rol, estado y búsqueda; paginado; formulario de alta |
| `#/admin/personas/{id}` | Datos y edición, baja con motivo, membresías (asignar, renovar, cancelar con `Idempotency-Key`) y asignaciones |
| `#/prof`, `#/nutri` | Mis alumnos o pacientes |

Archivos: `web/src/pages/admin/` (`types.ts`, `PeoplePage.tsx`, `PersonPage.tsx`),
`web/src/pages/MyStudentsPage.tsx`, `web/src/App.tsx` y `web/src/styles.css`.

### Configuración

`.env.example` y `deploy/docker-compose.yml` pasan `MEMBERS_ADMIN_EMAIL`,
`MEMBERS_ADMIN_PASSWORD` y `MEMBERS_ADMIN_NAME`. El administrador se crea una sola vez
al arrancar con la identidad habilitada y nunca se sobrescribe. En Vercel hay que
cargar esas variables con valores propios.

## 2. Decisiones

- **D-78** (aprobada el 09/11/2026): reglas del padrón. Activación por correo para
  las altas del administrador, completar al usuario externo del Club con el mismo
  email, DNI de 7 u 8 dígitos, fechas de nacimiento entre 1900 y ayer, sin edad mínima.
- **D-79** (aprobada): Redis solo en local; se implementa en la etapa 12.
- **D-80** (propuesta): ruteo del frontend por hash, de la etapa 2.

El Word de decisiones no se regeneró por falta de disco. Hay que correr
`scripts/sync-club-docs.ps1` como indica [PLAN_CLUB_BENEFICIOS.md](../PLAN_CLUB_BENEFICIOS.md).

## 3. Tests escritos

| Test | Cubre |
|---|---|
| `model.TestMembresiaCubreFecha` | Qué membresía cubre una fecha, límites inclusivos |
| `repository.TestPadronPostgres` (testcontainers) | Administrador inicial idempotente; alta con activación, outbox y auditoría (CA-01.1); duplicados y concurrencia (CA-01.2, RN-33); persona externa que pasa a alumna (D-78); profesionales y matrícula (CA-02.1, CA-02.2); validaciones del SPEC §9; baja lógica y login bloqueado (CA-01.3, CA-01.4, RN-34); asignaciones (CA-02.4, RN-30); búsquedas internas y vigencia |
| `server.TestMisAlumnosSoloProfesionales` (gateway) | Solo profesor y nutricionista llegan a mis alumnos |

## 4. Pendiente de otras etapas

- Rechazar la baja de un profesor con clases futuras (RN-34, CA-02.3) y que Booking
  consuma `usuario.desactivado`: requieren Booking (etapa 4) y mensajería (etapa 7).
- Publicar el outbox de usuarios en RabbitMQ: etapa 7.

## 5. Verificación

| Verificación | Resultado |
|---|---|
| `gofmt` de los archivos Go | Correcto |
| `go build`, `go vet`, `go test` | **No ejecutados**: disco lleno en el equipo de desarrollo |
| `npm run build` | **No ejecutado**: el equipo no tiene Node |

Lista para el integrante que prueba (Git Bash o PowerShell, desde la raíz):

- [ ] `cd services/members-service && go vet ./... && go test ./...` con Docker en marcha.
  Si Ryuk falla, repetir con `TESTCONTAINERS_RYUK_DISABLED=true`. `TestPadronPostgres`
  no debe saltearse (con `REQUIRE_DOCKER_TESTS=1` falla si no hay Docker).
- [ ] `cd gateway && go vet ./... && go test ./...`.
- [ ] `npm --prefix web ci && npm --prefix web run build`.
- [ ] `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/linea-base.ps1 -OmitirProduccion`.

Prueba manual con `make up`, identidad habilitada (`MEMBERS_IDENTITY_ENABLED=true`,
secretos y claves como en [CB03_VALIDACION.md](../CB03_VALIDACION.md)) y Mailpit en
<http://localhost:8025>:

- [ ] Login con `admin@gym.local` → `#/admin/personas`.
- [ ] Alta de un alumno; llega el correo de activación; el alumno elige contraseña.
- [ ] Un DNI o email repetido muestra "persona duplicada".
- [ ] Asignar una membresía al alumno; el alumno la ve en `#/app`.
- [ ] Alta de un profesor, asignarle el alumno; el profesor lo ve en `#/prof`.
- [ ] Revocar la asignación: el alumno desaparece de `#/prof`.
- [ ] Dar de baja al alumno: no puede volver a iniciar sesión.

**Aceptación de la etapa:** un administrador prepara un alumno con membresía válida y
el alumno puede consultarla.
