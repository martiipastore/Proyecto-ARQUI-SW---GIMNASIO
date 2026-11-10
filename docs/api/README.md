# APIs del gimnasio (`/api/v1`)

Contratos de las APIs que usa el frontend del gimnasio a través del gateway, y de las
APIs internas entre servicios. La API pública para otros grupos es otra: está en
[docs/contracts/](../contracts/README.md).

| Documento | Servicio | Estado |
|---|---|---|
| [auth.v1.md](auth.v1.md) | members-service: activación, login, logout y cuenta propia | Implementado (CB-03) |
| [memberships.v1.md](memberships.v1.md) | members-service: tipos, asignación, renovación y cancelación | Implementado (CB-04, D-77) |
| [users.v1.md](users.v1.md) | members-service: alumnos, profesionales, asignaciones y APIs internas | Implementado en la etapa 3 de la entrega final; pendiente de prueba con Docker |
| [booking.v1.md](booking.v1.md) | booking-service: actividades, horarios, clases, reservas e ingreso | Diseñado en la etapa 1 de la entrega final |
| [benefits.v1.md](benefits.v1.md) | benefits-service: saldo y movimientos propios, administración del Club | Parcial: `GET /benefits/me` implementado |
| [training.v1.md](training.v1.md) | training-service: planes, mediciones y consultas nutricionales | Diseñado en la etapa 1 de la entrega final |

Un contrato "diseñado" es el acuerdo que siguen frontend y backend al implementar la
etapa correspondiente. Si la implementación necesita cambiarlo, se actualiza el
documento en el mismo PR.

## Convenciones comunes

**Autenticación.** Toda ruta lleva `Authorization: Bearer <JWT>`, salvo las de acceso
público que indique cada contrato. El gateway valida el token, borra cualquier
`X-User-Id`, `X-User-Role` o `X-Internal-*` del cliente e inyecta la identidad verificada
(ADR-006). El gateway aplica una autorización gruesa por rol; el servicio dueño del dato
vuelve a autorizar sobre el recurso (RN-30). Las columnas "Quién" indican el rol requerido.

**Formato.** JSON en `snake_case` y en español. Fechas de calendario `AAAA-MM-DD`;
instantes en RFC 3339 UTC. Las reglas de "hoy" se evalúan en America/Argentina/Buenos_Aires.
Cuerpos de hasta 16 KiB, sin campos desconocidos (`400 VALIDACION`).

**Listados.** Paginación con `pagina` (desde 1) y `tamano` (1 a 100, por defecto 20).
Respuesta: `{"items": [...], "total": n, "pagina": p, "tamano": t}`.

**Idempotencia.** Las escrituras sensibles (reservar, cancelar, ingreso, asignar o
renovar membresía) aceptan `Idempotency-Key` de 8 a 128 caracteres `[A-Za-z0-9._:-]`.
Misma clave y mismo cuerpo: respuesta original con `Idempotent-Replayed: true`. Misma
clave y otro cuerpo: `409 CONFLICTO_IDEMPOTENCIA`. Primera ejecución todavía en curso:
`409 OPERACION_EN_CURSO`, reintentable con la misma clave.

**Errores.** RFC 7807 (`application/problem+json`) con `type`, `title`, `status`,
`detail`, `instance`, `code` y `correlation_id`. Los `code` salen del catálogo de
[ARCHITECTURE §12.1](../ARCHITECTURE.md#121-errores--rfc-7807-applicationproblemjson).
Errores comunes a todas las rutas:

| HTTP | `code` | Qué muestra el frontend |
|---|---|---|
| 400 | `VALIDACION` | Errores junto a cada campo |
| 401 | `NO_AUTENTICADO` | Volver al login |
| 403 | `SIN_PERMISO` | Pantalla de acceso denegado |
| 404 | `NO_ENCONTRADO` | Estado vacío o "no existe" |
| 409 | `CONFLICTO_IDEMPOTENCIA` / `OPERACION_EN_CURSO` | Reintentar con la misma clave |
| 429 | `LIMITE_EXCEDIDO` | Esperar `Retry-After` |
| 502/503/504 | `DEPENDENCIA_NO_DISPONIBLE` / `TIMEOUT` | Error temporal; en escrituras, consultar el estado antes de repetir |

**APIs internas** (`/internal/v1/...`): solo entre servicios, con
`X-Internal-Service-Key`. El gateway responde 404 a cualquier ruta con un segmento
`internal`. Datos personales sensibles, como el DNI, viajan en el cuerpo y nunca en la
URL, para que no queden en logs de acceso.
