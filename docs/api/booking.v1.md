# API de clases y reservas — v1 (diseño)

Servicio: booking-service (Hexagonal + CQRS de lectura). Base: `<gateway>/api/v1`.
Historias: HU-06 a HU-08, HU-13 a HU-16, HU-21, HU-22 y HU-35. Reglas: RN-07 a RN-21,
RN-32, RN-38 a RN-40 y D-35 a D-39. Convenciones comunes en [README.md](README.md).
Implementación: etapa 4 de la entrega final; la búsqueda pasa a OpenSearch en la etapa 8
sin cambiar este contrato.

## Actividades y horarios

| Método y ruta | Quién | Qué hace |
|---|---|---|
| GET /activities | Cualquier rol | Catálogo: 5 actividades iniciales (CA-06.1) |
| POST /activities | ADMINISTRADOR | Alta de actividad: `nombre` único, `capacidad_maxima` 1 a 30 (RN-08) |
| PUT /activities/{id} | ADMINISTRADOR | Cambia `nombre`, `capacidad_maxima` (tope 50 Musculación / 30 resto, CA-06.2) o `activa`; no inactiva con clases futuras reservadas (CA-06.3) |
| GET /activities/{id}/schedules | ADMINISTRADOR | Horarios recurrentes |
| POST /activities/{id}/schedules | ADMINISTRADOR | Horario: `dia_semana` (1–7), `inicio` (HH:MM), `duracion_minutos`, `profesor_id`, `capacidad`. Musculación solo en sus 7 franjas (CA-07.3); responsable PROFESOR activo y sin superposición (CA-07.4, CA-07.6) |
| POST /classes/generate | ADMINISTRADOR | Genera clases entre `desde` y `hasta` (fechas); idempotente: no duplica (CA-07.5) |

Los puntos por asistencia son fijos en 500 y no se configuran por actividad (RN-20, CA-06.4).

| HTTP | `code` | Cuándo |
|---|---|---|
| 400 | `VALIDACION` | Capacidad fuera de tope, franja de Musculación inválida o responsable que no es un PROFESOR activo |
| 409 | `SUPERPOSICION_PROFESOR` | El profesor ya es responsable de una clase superpuesta (RN-08, CA-07.4) |
| 409 | `TRANSICION_INVALIDA` | Inactivar una actividad con clases futuras reservadas (CA-06.3) |

## Clases

| Método y ruta | Quién | Qué hace |
|---|---|---|
| GET /classes | Cualquier rol | Búsqueda (HU-13): `fecha_desde`, `fecha_hasta`, `actividad_id`, `franja`, `solo_disponibles`, paginación; orden por `inicio` ascendente e `id` como desempate (ADR-014) |
| GET /classes/{id} | Cualquier rol; con `reservas` para ADMINISTRADOR y el profesor responsable | Detalle; para administración, asistentes y ausentes (CA-21.1, CA-35.1/2) |
| GET /classes/mine | PROFESOR | Mis clases con ocupación (HU-21) |
| PUT /classes/{id}/capacity | ADMINISTRADOR | `capacidad` ≥ ocupación y ≤ tope; solo PROGRAMADA (CA-08.1/2/4) |
| POST /classes/{id}/cancel | ADMINISTRADOR | `motivo`; cancela la clase y sus reservas activas (CA-08.3, RN-32) |

`Clase` en listados:

```json
{"id":"…","actividad":{"id":"…","nombre":"Funcional"},"inicio":"2026-11-10T12:00:00Z","fin":"2026-11-10T13:00:00Z","franja":null,"profesor":{"id":"…","nombre":"Lucía Gómez"},"capacidad":30,"ocupacion":21,"disponibles":9,"estado":"PROGRAMADA","reservable":true,"reservada_por_mi":false}
```

El frontend muestra `21/30 ocupados - 9 disponibles` (CA-13.1). `reservable` es falso si
la clase está completa, cancelada o ya iniciada (CA-13.2/3); `reservada_por_mi` marca
"ya reservada" (CA-13.4). La ocupación del listado puede estar levemente atrasada
(CQRS, L-03): la reserva siempre se valida contra PostgreSQL.

## Reservas

| Método y ruta | Quién | Qué hace |
|---|---|---|
| POST /bookings | ALUMNO | Reserva `{"clase_id":"…"}` con `Idempotency-Key` (HU-14) |
| GET /bookings/me | ALUMNO | Futuras y historial; filtro `estado`; paginación (HU-16) |
| POST /bookings/{id}/cancel | ALUMNO (propia) o ADMINISTRADOR | Cancela con `Idempotency-Key`; hasta 1 hora antes (HU-15, D-02, D-15) |

`Reserva`:

```json
{"id":"…","clase":{"id":"…","actividad":"Funcional","inicio":"2026-11-10T12:00:00Z","fin":"2026-11-10T13:00:00Z"},"estado":"CONFIRMADA","creada_en":"…","ingreso_en":null,"cancelada_en":null,"motivo_cancelacion":null}
```

Errores de reservar y cancelar (catálogo de ARCHITECTURE §12.1):

| HTTP | `code` | Regla |
|---|---|---|
| 422 | `MEMBRESIA_NO_VIGENTE` | Sin membresía vigente hoy o en la fecha de la clase (CA-14.2/3) |
| 422 | `CLASE_NO_FUTURA` / `CLASE_CANCELADA` | CA-14.4 |
| 409 | `SIN_CUPO` | CA-14.5, CA-14.8 |
| 409 | `RESERVA_DUPLICADA` | CA-14.6 |
| 409 | `TURNO_MUSCULACION_DIARIO` | CA-14.9 (D-17) |
| 409 | `SUPERPOSICION_RESERVA` | CA-14.11 (D-18) |
| 422 | `PLAZO_CANCELACION_VENCIDO` | CA-15.3 |
| 200 | — | Cancelar una reserva ya cancelada informa `estado: CANCELADA` sin cambios (CA-15.2) |
| 503 | `DEPENDENCIA_NO_DISPONIBLE` | Members no confirma la membresía: falla cerrada, no se reserva (ADR-005 v2) |

## Ingreso por DNI (HU-22, D-36 a D-39)

`POST /check-ins` con `{"dni":"40123456"}` y `Idempotency-Key`, desde la pantalla de
recepción. Lo usa el ADMINISTRADOR (D-82); no hay credencial de terminal ni rol propio.

| Resultado | HTTP | Cuerpo |
|---|---|---|
| Ingreso válido en la ventana de una reserva (D-37) | 201 | `{"resultado":"VERDE","mensaje":"Bienvenido","nombre":"Martina Pastore","membresia":{"estado":"VIGENTE","vence":"…"},"clase":{…}}`; la reserva pasa a ASISTIDA y se publica `asistencia.registrada` |
| Reingreso a una clase ya asistida (D-38) | 200 | `{"resultado":"VERDE","mensaje":"Bienvenido de nuevo",…}`; sin puntos |
| Membresía vencida | 422 `MEMBRESIA_NO_VIGENTE` | Problema con `nombre` y `vencida_el` (D-36) |
| Sin reserva con ventana abierta (D-39) | 422 `SIN_RESERVA_PARA_HORARIO` | Problema con `nombre` |
| Persona inactiva | 422 `USUARIO_INACTIVO` | Problema con `nombre` |
| DNI inexistente | 404 `NO_ENCONTRADO` | Sin datos de nadie (CA-22.7) |

En la terminal, el frontend muestra el nombre en verde o en rojo con el motivo. Al
terminar la clase, las reservas sin ingreso pasan a AUSENTE y se publica
`inasistencia.registrada` (CA-22.8).
