# API de usuarios del gimnasio — v1 (diseño)

Servicio: members-service (Capas). Base: `<gateway>/api/v1`. Historias: HU-01, HU-02,
HU-21 (mis alumnos), RN-30, RN-33, RN-34 y RN-36. Convenciones comunes en
[README.md](README.md). Implementación: etapa 3 de la entrega final.

## Endpoints

| Método y ruta | Quién | Qué hace | Respuesta |
|---|---|---|---|
| GET /users | ADMINISTRADOR | Lista personas; filtros `rol`, `estado`, `q` (DNI exacto o texto en nombre, apellido o email) | 200 página de `Persona` |
| POST /users | ADMINISTRADOR | Alta de alumno o profesional (HU-01, HU-02) | 201 `Persona` |
| GET /users/{id} | ADMINISTRADOR o la propia persona | Detalle y asignaciones | 200 `{persona, asignaciones}` |
| PUT /users/{id} | ADMINISTRADOR | Modifica datos personales | 200 `Persona` |
| POST /users/{id}/deactivate | ADMINISTRADOR | Baja lógica con `motivo` (RN-34, CA-01.3) | 200 `Persona` |
| POST /users/{id}/assignments | ADMINISTRADOR | Asigna el alumno `alumno_id` al profesional `{id}` (CA-02.4) | 201 `Asignacion` |
| POST /users/{id}/assignments/{alumnoId}/revoke | ADMINISTRADOR | Revoca la asignación vigente | 200 `Asignacion` |
| GET /users/me/students | PROFESOR, NUTRICIONISTA | Mis alumnos o pacientes asignados (HU-21) | 200 `{items: [Asignacion]}` |
| GET /users/me | Cualquier rol | Ya existe ([auth.v1.md](auth.v1.md)); se le agrega `persona` con los datos del padrón | 200 |

## Alta

```json
{"rol":"ALUMNO","nombre":"Martina","apellido":"Pastore","dni":"40123456","email":"martina@example.test","telefono":"+54 351 555-0000","fecha_nacimiento":"2001-05-10"}
```

| Campo | Regla (SPEC §9) |
|---|---|
| `rol` | `ALUMNO`, `PROFESOR` o `NUTRICIONISTA`. El administrador no se crea por esta API |
| `nombre`, `apellido` | 1 a 80 caracteres, no solo espacios |
| `dni` | 7 u 8 dígitos; único (RN-33) |
| `email` | Formato válido, normalizado a minúsculas; único (RN-33) |
| `telefono` | Opcional; dígitos, espacios, `+`, `-` y paréntesis, 6 a 30 caracteres |
| `fecha_nacimiento` | Obligatoria para alumnos; anterior a hoy y no anterior a 1900; sin edad mínima |
| `matricula` | Opcional y solo para `NUTRICIONISTA` |

Efectos del alta:

- La persona queda `ACTIVO` y recibe por correo un enlace de activación de un solo uso
  para elegir su contraseña (mismo mecanismo que el registro externo, ADR-017). Nadie
  conoce ni envía contraseñas.
- Si ya existe una persona externa del Club (sin DNI) con el mismo email y se da de alta
  como alumno, se completa ese mismo usuario: conserva cuenta, puntos y contraseña.
  Cualquier otro email o DNI repetido es `409 PERSONA_DUPLICADA` (CA-01.2).
- Un alta de alumno escribe `alumno.creado` en el outbox (CA-01.1, S-10).

`Persona`:

```json
{"id":"…","rol":"ALUMNO","nombre":"Martina","apellido":"Pastore","dni":"40123456","email":"martina@example.test","telefono":"+54 351 555-0000","fecha_nacimiento":"2001-05-10","matricula":null,"estado":"ACTIVO","registro_externo":false,"fecha_alta":"2026-11-09T15:00:00Z","dado_de_baja_en":null,"activacion_pendiente":true}
```

## Modificación y baja

- `PUT /users/{id}` acepta `nombre`, `apellido`, `telefono`, `fecha_nacimiento` y
  `matricula`. DNI, email y rol no se modifican.
- `POST /users/{id}/deactivate` con `{"motivo":"…"}` (3 a 200 caracteres) deja la persona
  `INACTIVO`; nunca se borra. Escribe `usuario.desactivado` en el outbox, que booking
  consumirá para cancelar reservas futuras (D-20, CA-01.3). Una persona inactiva no
  puede iniciar sesión (CA-01.4). Dar de baja a un profesor con clases futuras se
  rechazará cuando exista booking (RN-34, CA-02.3).

## Asignaciones

`POST /users/{profesionalId}/assignments` con `{"alumno_id":"…"}`. Ambos deben estar
`ACTIVO`, el profesional debe ser profesor o nutricionista y no puede existir otra
asignación vigente igual (`409 ASIGNACION_DUPLICADA`). Revocar cierra la asignación
desde ese momento, sin período de gracia (RN-30); la historia se conserva.

`Asignacion`:

```json
{"id":"…","profesional_id":"…","alumno_id":"…","desde":"2026-11-09T15:00:00Z","hasta":null,"alumno":{"id":"…","nombre":"Martina","apellido":"Pastore","email":"…","telefono":"…","rol":"ALUMNO","estado":"ACTIVO"}}
```

## Errores propios

| HTTP | `code` | Cuándo |
|---|---|---|
| 409 | `PERSONA_DUPLICADA` | DNI o email ya registrados (RN-33) |
| 409 | `ASIGNACION_DUPLICADA` | Ya existe una asignación vigente igual |
| 409 | `TRANSICION_INVALIDA` | Dar de baja a alguien ya inactivo |
| 400 | `VALIDACION` | Asignar con roles incorrectos o una persona inactiva |

## APIs internas (`/internal/v1`)

| Método y ruta | Consumidor | Respuesta |
|---|---|---|
| GET /internal/v1/alumnos?ids=a,b | booking (listados, HU-21) | `{items:[{id,nombre,apellido,estado}]}`; hasta 100 ids |
| POST /internal/v1/alumnos/buscar-dni con `{"dni":"…"}` | booking (ingreso por DNI, HU-22) | `{id,nombre,apellido,estado}` o 404 |
| GET /internal/v1/asignaciones?profesional_id=&alumno_id= | training (RN-30) | `{"vigente":true}`; sin caché (D-33) |
| GET /internal/v1/alumnos/{id}/vigencia?fechas=AAAA-MM-DD,… | booking (reservar, RN-12) | `{vigente_hoy, vigente_en:{fecha:{vigente, membresia_id}}, evaluado_en}` (ARCHITECTURE §4.2) |
