# API del Club de Beneficios para la aplicación — v1

Servicio: benefits-service (Hexagonal). Base: `<gateway>/api/v1`. Es la API de la
aplicación del gimnasio; la de otros grupos es la [API de partners v2](../contracts/README.md).
Historias: HU-10, HU-11, HU-19, HU-35 y HU-CB-07. Catálogo, canjes y débitos quedan
fuera por D-76. Convenciones comunes en [README.md](README.md).

## Endpoints

| Método y ruta | Quién | Qué hace | Estado |
|---|---|---|---|
| GET /benefits/me | Cualquier rol con cuenta | Saldo propio: `{"balance":…, "updatedAt":…}` | Implementado (CB-03) |
| GET /benefits/me/movements | Cualquier rol con cuenta | Movimientos propios paginados (HU-19) | Implementado localmente, etapa 6 (10/11/2026) |
| GET /benefits/accounts/{usuarioId} | ADMINISTRADOR | Saldo, vinculaciones y movimientos de una persona (HU-CB-07, HU-35) | Implementado localmente, etapa 7 |
| POST /benefits/movements/{id}/revert | ADMINISTRADOR | Revierte con `motivo` e `Idempotency-Key` (HU-11) | Implementado localmente, etapa 7 |
| GET /benefits/partners | ADMINISTRADOR | Integraciones con estado, permisos y límites | Implementado localmente, etapa 7 |
| POST /benefits/partners | ADMINISTRADOR | Alta de integración; devuelve la API key **una sola vez** (CA-10.1) | Implementado localmente, etapa 7 |
| POST /benefits/partners/{id}/rotate-key | ADMINISTRADOR | Nueva API key; la anterior deja de funcionar | Implementado localmente, etapa 7 |
| PUT /benefits/partners/{id} | ADMINISTRADOR | `activo`, permisos y límites (ADR-016) | Implementado localmente, etapa 7 |
| POST /benefits/partners/{id}/links | ADMINISTRADOR | Vincula la cuenta de una persona con un `external_user_id` (CA-10.3/10.4) | Implementado localmente, etapa 7 |
| GET /benefits/catalog | ALUMNO, ADMINISTRADOR | Beneficios canjeables hoy: activos y vigentes (RN-24) | Implementado localmente, D-94 |
| POST /benefits/redemptions | ALUMNO | Canjea `beneficio_id` con `Idempotency-Key` obligatoria (HU-20) | Implementado localmente, D-94 |
| GET /benefits/me/redemptions | ALUMNO | Cupones propios paginados | Implementado localmente, D-94 |
| GET /benefits/rewards | ADMINISTRADOR | Catálogo completo, con inactivos y cantidad de canjes (HU-09) | Implementado localmente, D-94 |
| POST /benefits/rewards | ADMINISTRADOR | Alta de beneficio | Implementado localmente, D-94 |
| PUT /benefits/rewards/{id} | ADMINISTRADOR | Reemplaza nombre, descripción, costo, vigencia y `activo` | Implementado localmente, D-94 |
| GET /benefits/redemptions | ADMINISTRADOR | Busca cupones por `codigo` y `estado`, paginado | Implementado localmente, D-94 |
| POST /benefits/redemptions/{id}/use | ADMINISTRADOR | Marca el cupón como USADO al entregar el beneficio | Implementado localmente, D-94 |
| POST /benefits/redemptions/{id}/cancel | ADMINISTRADOR | Anula un cupón CONFIRMADO con `motivo` y devuelve los puntos (RN-25) | Implementado localmente, D-94 |

`scripts/provision-club-partner.ps1` ([VERCEL_CLUB.md](../deploy/VERCEL_CLUB.md))
sigue sirviendo para operar directo sobre la base, por ejemplo para la primera
integración antes de que exista un administrador. Detalle de la etapa 7 en
[ETAPA_7_CLUB.md](../entrega-final/ETAPA_7_CLUB.md).

## Administración (etapa 7)

Todas las rutas exigen la identidad que inyecta el gateway y la clave interna.
Benefits vuelve a verificar el rol ADMINISTRADOR (`403 SIN_PERMISO`). Las
respuestas llevan `Cache-Control: no-store`.

### Integración

```json
{"id":"…","nombre":"Estudiario","activo":true,"estado":"ACTIVO","permite_consulta":true,"permite_puntos":true,"permite_registro":false,"requiere_evento":true,"max_puntos_operacion":1000000,"max_puntos_dia":null,"max_solicitudes_minuto":120,"vinculaciones":3,"creada_en":"2026-11-10T15:00:00Z"}
```

- `POST /benefits/partners` exige `nombre` (3 a 100 caracteres) y acepta los demás
  campos menos `activo`: el alta siempre queda ACTIVA. Valores por defecto:
  `permite_consulta`/`permite_puntos`/`requiere_evento` en `true`, `permite_registro`
  en `false`, 1.000.000 puntos por operación, sin tope diario y 120 solicitudes por
  minuto. Responde `201` con la integración y `api_key`, que no vuelve a mostrarse.
- `PUT /benefits/partners/{id}` reemplaza la configuración completa: todos los campos
  son obligatorios, salvo `max_puntos_dia` (`null` o ausente = sin tope). `200`.
- `POST /benefits/partners/{id}/rotate-key` sin cuerpo. Responde `200 {"id","api_key"}`.
- `POST /benefits/partners/{id}/links` con `{"usuario_id","external_user_id"}`.
  Responde `201` si crea el vínculo y `200` si ya existía igual. Devuelve
  `409 CONFLICTO_IDENTIDAD` si el identificador o la cuenta ya están vinculados de
  otra forma, y `404 CUENTA_NO_ENCONTRADA` si la persona todavía no tiene cuenta.

### Cuenta de una persona

`GET /benefits/accounts/{usuarioId}?pagina=1&tamano=20` responde
`{"usuario_id","saldo","creada_en","vinculaciones":[{"partner_id","partner","external_user_id"}],"items":[…],"total","pagina","tamano"}`.
Los ítems usan el formato de **Movimiento** y agregan `partner_id` y, si
corresponde, `revertido_por` (id del compensatorio).

### Reversión

`POST /benefits/movements/{id}/revert` con `{"motivo":"…"}` (3 a 200 caracteres) e
`Idempotency-Key` obligatoria (8 a 128 caracteres `A-Za-z0-9._:-`, única por
administrador). Responde `201` con `{"movimiento":{…,"origen":"REVERSION","revierte_id":"…"},"saldo":n}`.
Un reintento con la misma clave y los mismos datos devuelve `200` y
`Idempotent-Replayed: true`. Con la misma clave y otros datos responde
`409 CONFLICTO_IDEMPOTENCIA`. Un movimiento inexistente devuelve `404 NO_ENCONTRADO`.
La respuesta `/benefits/me` conserva sus claves actuales (`balance`, `updatedAt`) porque
ya la usa el frontend.

## Movimiento

```json
{"id":"…","fecha":"2026-11-10T13:05:00Z","motivo":"Asistencia a Funcional","puntos":500,"estado":"CONFIRMADO","origen":"ASISTENCIA","secuencia":12}
```

`origen`: `ASISTENCIA`, `INASISTENCIA`, `PARTNER`, `REVERSION` o `CANJE` (D-94). Las penalizaciones se
muestran con signo negativo, incluida la de 0 puntos con saldo 0 (CA-22.11). El saldo
mostrado es la suma de todos los movimientos (CA-19.2).

### Historial propio implementado (etapa 6)

`GET /api/v1/benefits/me/movements?pagina=1&tamano=20` responde
`{"items":[...],"total":n,"pagina":1,"tamano":20}`. `pagina`: 1 a 1.000.000;
`tamano`: 1 a 100. Defaults: 1 y 20. No admite otros parámetros, valores vacíos ni
repetidos; responde `400 VALIDACION`. La identidad se obtiene únicamente del
gateway autenticado, con clave interna en Benefits; no hay selector de usuario.

Los movimientos están ordenados por `secuencia DESC`, el orden de aplicación de
RN-42, y conservan `fecha` del hecho. También incluyen `aplicado_en`, `origen_id` y
`referencia`. Desde la etapa 7 también aparece `REVERSION`, con `revierte_id`, y el
original pasa a `estado: "REVERTIDO"`. El usuario ve toda su cuenta; la API de
partners conserva el filtro por integración. `401 NO_AUTENTICADO` sin identidad
confiable, `404 CUENTA_NO_ENCONTRADA` sin cuenta, `503 DEPENDENCIA_NO_DISPONIBLE`
si no se puede consultar la persistencia. Respuestas `Cache-Control: no-store`.

## Errores propios

| HTTP | `code` | Cuándo |
|---|---|---|
| 404 | `CUENTA_NO_ENCONTRADA` | La cuenta todavía no existe (alta reciente, S-10) |
| 422 | `SALDO_INSUFICIENTE` | Revertir dejaría saldo negativo (CA-11.3) o el saldo no alcanza para canjear (CA-20.2) |
| 422 | `BENEFICIO_NO_DISPONIBLE` | Canjear un beneficio inactivo o fuera de vigencia (CA-20.3) |
| 409 | `TRANSICION_INVALIDA` | Revertir un movimiento ya revertido, una reversión (CA-11.2) o un débito de canje; usar o anular un cupón que no está CONFIRMADO |
| 409 | `CONFLICTO_IDENTIDAD` | Vínculo externo incompatible con el existente (CA-10.4) |
| 409 | `CONFLICTO_IDEMPOTENCIA` | Idempotency-Key reutilizada con otros datos |
