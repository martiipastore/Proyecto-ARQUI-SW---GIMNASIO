# API de membresías del gimnasio — v1.0.0

Implementada en CB-04 (ADR-020) y precisada por D-77. Base pública: `<gateway>/api/v1`.
Todas las rutas requieren `Authorization: Bearer <JWT>`; el gateway verifica el
token e inyecta la identidad (ADR-006). Members nunca toma el actor del body.

## Endpoints

| Método y ruta | Quién | Solicitud y respuesta |
|---|---|---|
| GET /memberships/types | Cualquier rol | 200 con `items`: tipos y su configuración vigente |
| GET /memberships/me | Cualquier rol | 200 con la membresía vigente e historial propio, paginado con `pagina` y `tamano` (1–100, por defecto 20) |
| GET /memberships/users/{userID} | ADMINISTRADOR | Igual que `/me`, para otro usuario |
| POST /memberships | ADMINISTRADOR | Asigna una membresía; 201 |
| POST /memberships/users/{userID}/renew | ADMINISTRADOR | Renueva; 201 |
| POST /memberships/{id}/cancel | ADMINISTRADOR | Cancela una membresía ACTIVA con `motivo`; 200 |
| PUT /memberships/types/{codigo} | ADMINISTRADOR | Configura un tipo; 200 |

Las escrituras exigen `Idempotency-Key` (8 a 128 caracteres `[A-Za-z0-9._:-]`).
Repetirla con el mismo cuerpo devuelve el resultado original con
`Idempotent-Replayed: true`; con otro cuerpo, `409 CONFLICTO_IDEMPOTENCIA`.
Cuerpo JSON de hasta 16 KiB, sin campos desconocidos.

## Asignar y renovar

```json
{"usuario_id":"<uuid>","tipo_codigo":"MENSUAL","fecha_inicio":"2026-11-09","motivo":"Alta en recepción"}
```

| Campo | Regla |
|---|---|
| `usuario_id` | UUID de un usuario con rol **ALUMNO**; otro rol responde 400 (HU-03) |
| `tipo_codigo` | `MENSUAL`, `TRIMESTRAL`, `ANUAL` o `CLASES`, habilitado |
| `fecha_inicio` | Solo al asignar; hoy o posterior (D-12). Al renovar se calcula (RN-02) y no se envía |
| `fecha_vencimiento` | **Opcional.** Si se omite, se calcula según el tipo; si se envía, debe coincidir con ese cálculo (D-77). Solo es obligatoria para un tipo por clases sin duración configurada |
| `motivo` | 3 a 200 caracteres |

Cálculo del vencimiento (D-77, CA-03.1 y S-04): el día de vencimiento es vigente,
así que el período termina el día anterior al mismo día del período siguiente.

| Tipo | Inicio | Vencimiento |
|---|---|---|
| Mensual (1 mes) | 09/11/2026 | 08/12/2026 |
| Trimestral (3 meses) | 09/11/2026 | 08/02/2027 |
| Anual (12 meses) | 09/11/2026 | 08/11/2027 |
| Mensual, mes destino más corto | 31/01/2027 | 28/02/2027 (último día del mes) |
| Por días (N) | 09/11/2026, N = 30 | 08/12/2026 (inicio + N − 1) |

La duración de cada tipo es la configurada al momento del otorgamiento; editar el
tipo después no modifica membresías anteriores (se guarda una copia).

## Configurar un tipo

```json
{"nombre":"Mensual","duracion_meses":1,"habilitado":true,"motivo":"Ajuste comercial"}
```

`duracion_meses` y `duracion_dias` son excluyentes. Mensual, trimestral y anual
deben definir una de las dos. `clases` solo se admite en `CLASES`, que puede quedar
sin duración; en ese caso, cada asignación debe indicar `fecha_vencimiento`.

## Errores

| HTTP | `code` | Cuándo |
|---|---|---|
| 400 | `VALIDACION` | Tipo, fechas, rol del destinatario, motivo o clave inválidos |
| 401 | `NO_AUTENTICADO` | JWT ausente o inválido |
| 403 | `SIN_PERMISO` | El rol no permite la operación |
| 404 | `MEMBRESIA_NO_ENCONTRADA` | Usuario, tipo o membresía inexistente |
| 409 | `MEMBRESIAS_SUPERPUESTAS` | Se superpone con otra ACTIVA (RN-02) |
| 409 | `TRANSICION_MEMBRESIA_INVALIDA` | Cancelar una membresía no ACTIVA, renovar sin historial |
| 409 | `CONFLICTO_IDEMPOTENCIA` / `OPERACION_EN_CURSO` | Clave reutilizada o en curso |
| 503 | `DEPENDENCIA_NO_DISPONIBLE` | Base no disponible |

Una membresía vencida o cancelada no modifica el saldo ni los movimientos de puntos
(RN-CB-02, D-66).
