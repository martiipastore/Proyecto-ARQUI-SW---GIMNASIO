# Club de Beneficios — Guía para integrar la API de fidelización

Esta guía es para equipos de **otros sistemas** que quieran sumar puntos de fidelización a su flujo: acreditar puntos cuando un cliente compra o cumple un turno, debitarlos cuando los usa, canjearlos por beneficios y mostrar su saldo.

| | |
|---|---|
| **Contrato** | [benefits-api.v1.yaml](benefits-api.v1.yaml) (OpenAPI 3.1) |
| **Versión vigente** | **v1.0.0** |
| **Mock local** | `http://localhost:4010` (ver [Probar con el mock](#probar-con-el-mock)) |
| **Producción** | `https://<host>/partner-api` — *URL pública a confirmar (proveedor de nube sin definir)* |
| **Formato** | JSON (UTF-8); errores en `application/problem+json` (RFC 7807) |

---

## Qué ofrece

| Operación | Método y ruta | Para qué |
|---|---|---|
| Acreditar puntos | `POST /v1/accounts/{externalUserId}/credits` | Premiar algo que hizo el usuario en tu sistema |
| Debitar puntos | `POST /v1/accounts/{externalUserId}/debits` | Descontar puntos que el usuario usa en tu negocio |
| Consultar saldo | `GET /v1/accounts/{externalUserId}/balance` | Mostrar cuántos puntos tiene |
| Listar movimientos | `GET /v1/accounts/{externalUserId}/movements?page=1&size=20` | Mostrar el historial de lo que hiciste con esa cuenta |
| Ver catálogo | `GET /v1/benefits` | Ofrecer beneficios canjeables |
| Canjear beneficio | `POST /v1/accounts/{externalUserId}/redemptions` | Cambiar puntos por un beneficio del catálogo |

### Conceptos clave

- **Cuenta = (partner, `externalUserId`).** El `externalUserId` es **tu** identificador del usuario (1 a 64 caracteres: letras, números y `. _ : @ -`). No necesitás conocer IDs internos del gimnasio.
- **Las cuentas se vinculan antes de usarse.** Las personas son alumnos del gimnasio; el administrador del gimnasio vincula cada alumno con tu `externalUserId`. Si operás sobre un usuario no vinculado a vos recibís `404 ACCOUNT_NOT_FOUND`.
- **Saldo compartido, historial propio.** El saldo es el total de la cuenta (incluye, por ejemplo, puntos que el alumno ganó por asistir a clases). En `movements` ves **solo los movimientos que hiciste vos**.
- **Los movimientos son inmutables.** No hay endpoint para borrar ni editar. Si hubo un error, el gimnasio lo corrige con un movimiento compensatorio (`REVERSAL`) y el original queda `REVERTED`.
- **El saldo nunca es negativo.** Un débito o canje que no alcanza se rechaza entero.

## Cómo obtener una API key

1. Pedí el alta como partner a cualquier integrante del equipo (ver [Equipo](../../README.md#equipo)) o abriendo un *issue* en este repositorio con el título `Alta de partner: <nombre de tu sistema>`. Indicá un contacto técnico.
2. El administrador del gimnasio crea el partner y te entrega la **API key una sola vez**. Guardala como secreto (variable de entorno o gestor de secretos); nunca en el código ni en el frontend.
3. Coordiná la **vinculación de usuarios**: para cada alumno que también sea usuario tuyo, el administrador registra tu `externalUserId`.
4. Si la key se filtra, pedí que se revoque; se emite una nueva y la anterior deja de funcionar de inmediato.

> En el **mock** cualquier valor de `X-API-Key` es aceptado y cualquier `externalUserId` responde.

## Autenticación

Mandá la key en cada request:

```http
X-API-Key: <tu-api-key>
```

| Situación | Respuesta |
|---|---|
| Falta el header o la key es inválida/revocada | `401 UNAUTHORIZED` |
| Partner suspendido | `403 FORBIDDEN` |

Opcional: mandá `X-Correlation-Id` (UUID) para rastrear una solicitud en los logs de ambos sistemas. Siempre se devuelve en la respuesta; incluilo si reportás un problema.

## Idempotencia

Las operaciones que modifican puntos (`credits`, `debits`, `redemptions`) **exigen** el header `Idempotency-Key`. Así podés reintentar ante un timeout sin acreditar dos veces.

| Caso | Resultado |
|---|---|
| Key nueva | Se procesa la operación |
| Misma key + **mismo body** | Se devuelve **la respuesta original** (mismo status y body) con `Idempotent-Replayed: true`. No se aplica de nuevo. |
| Misma key + **body distinto** | `409 IDEMPOTENCY_CONFLICT` |
| Misma key mientras la primera todavía se procesa | `409 IDEMPOTENCY_IN_PROGRESS` → reintentá en unos segundos con la misma key |
| Reintento de una operación que fue rechazada (ej. saldo insuficiente) | Se devuelve **el mismo rechazo**. Para intentar de nuevo, usá una key nueva. |

Reglas prácticas:

- Generá **una key por operación lógica** (recomendado: UUID v4) y guardala junto a tu registro de origen (la compra, el turno). Entre 8 y 128 caracteres: letras, números y `. _ : -`.
- Las keys son únicas **por partner** y se conservan **30 días**. Después de ese plazo, la misma key se trata como nueva.
- Las consultas (`GET`) son seguras de reintentar y no usan key.

## Errores

Todos los errores tienen este formato:

```json
{
  "type": "https://gimnasio.example.com/problems/insufficient-balance",
  "title": "Saldo insuficiente",
  "status": 422,
  "detail": "Se pidieron 5000 puntos y el saldo disponible es 770.",
  "instance": "/v1/accounts/cli-10045/debits",
  "code": "INSUFFICIENT_BALANCE",
  "correlationId": "3c5e7a9b-1d2f-4a6b-8c0d-2e4f6a8b0c1d"
}
```

Programá contra el campo **`code`** (estable); `title` y `detail` son texto para personas y pueden cambiar.

| `code` | HTTP | Qué significa | ¿Reintentar? |
|---|---|---|---|
| `INVALID_REQUEST` | 400 | Body, parámetros o headers inválidos. El array `errors` indica cada campo. | **No.** Corregí la solicitud. |
| `UNAUTHORIZED` | 401 | API key ausente, inválida o revocada. | **No.** Revisá la credencial. |
| `FORBIDDEN` | 403 | Partner suspendido o sin permiso. | **No.** Contactá al gimnasio. |
| `ACCOUNT_NOT_FOUND` | 404 | El `externalUserId` no está vinculado a tu partner. | **No.** Pedí la vinculación. |
| `BENEFIT_NOT_FOUND` | 404 | El `benefitId` no existe. | **No.** Volvé a leer el catálogo. |
| `IDEMPOTENCY_CONFLICT` | 409 | Reusaste una key con otro body. | **No.** Usá una key nueva si es otra operación. |
| `IDEMPOTENCY_IN_PROGRESS` | 409 | La operación original con esa key todavía está en curso. | **Sí**, con la misma key, tras 1–2 s. |
| `INSUFFICIENT_BALANCE` | 422 | El saldo no alcanza. No se aplicó nada. | **No** con la misma key. |
| `BENEFIT_NOT_AVAILABLE` | 422 | Beneficio vencido, inactivo o no habilitado para partners. | **No.** Volvé a leer el catálogo. |
| `RATE_LIMITED` | 429 | Superaste el límite de uso. | **Sí**, después de `Retry-After` segundos. |
| `INTERNAL` | 500 | Error inesperado de nuestro lado. | **Sí**, con backoff exponencial y la **misma key**. |
| `UNAVAILABLE` | 503 | Servicio caído o en mantenimiento. | **Sí**, después de `Retry-After`, con la **misma key**. |
| *(timeout / error de red)* | — | No sabés si se aplicó. | **Sí**, con la **misma key**: si ya se aplicó, recibís la respuesta original. |

**Backoff recomendado:** 1 s, 2 s, 4 s, 8 s (máximo 5 intentos), con un poco de aleatoriedad. Siempre respetá `Retry-After` cuando viene.

## Límites de uso

| Límite | Valor |
|---|---|
| Solicitudes por API key | 20 por segundo, ráfagas de hasta 40 |
| Puntos por operación | 1 a 1.000.000 (entero) |
| `reason` | 3 a 200 caracteres |
| `reference` | 1 a 100 caracteres |
| Página de movimientos (`size`) | 1 a 100 (por defecto 20) |
| Retención de `Idempotency-Key` | 30 días |
| Timeout del lado del servidor | 5 s |

Los límites de tasa pueden ajustarse; cualquier cambio se anuncia en el [CHANGELOG](#changelog-del-contrato).

## Probar con el mock

El mock (Prism) responde con los **ejemplos del contrato** y valida tus requests contra el esquema. No guarda estado: el saldo no cambia entre llamadas.

Requisito: Docker. Desde la raíz del repositorio:

```bash
docker compose -f deploy/docker-compose.mock.yml up -d     # levanta el mock en http://localhost:4010
docker compose -f deploy/docker-compose.mock.yml logs -f   # ver requests y validaciones
docker compose -f deploy/docker-compose.mock.yml down      # bajarlo
```

Con `make` instalado: `make mock-up`, `make mock-logs`, `make mock-down`.

Para pedir una respuesta de error concreta, usá el header `Prefer` de Prism:

| Header | Efecto |
|---|---|
| `Prefer: code=422` | Devuelve la respuesta 422 de la operación |
| `Prefer: example=idempotencyInProgress` | Devuelve el ejemplo con ese nombre |
| `Prefer: dynamic=true` | Genera datos aleatorios válidos según el esquema |

## Ejemplos paso a paso

Las variables de abajo apuntan al mock. Para producción, cambiá `BASE` y `API_KEY`.

```bash
BASE=http://localhost:4010
API_KEY=demo-partner-key
USER_ID=cli-10045
```

### 1. Ver el catálogo de beneficios

```bash
curl -s "$BASE/v1/benefits" -H "X-API-Key: $API_KEY"
```

### 2. Acreditar puntos por una compra

```bash
curl -s -X POST "$BASE/v1/accounts/$USER_ID/credits" \
  -H "X-API-Key: $API_KEY" \
  -H "Idempotency-Key: 9f1b6c2e-3a4d-4e5f-8a9b-0c1d2e3f4a5b" \
  -H "Content-Type: application/json" \
  -d '{"points": 150, "reason": "Compra en tienda online", "reference": "ORDER-A-7781"}'
```

Respuesta `201`:

```json
{
  "movementId": "7d0c4a8e-2f4b-4c51-9e0e-3c2b8f1a9d10",
  "externalUserId": "cli-10045",
  "type": "CREDIT",
  "points": 150,
  "reason": "Compra en tienda online",
  "reference": "ORDER-A-7781",
  "occurredAt": "2026-10-07T18:42:10Z",
  "balance": 1270
}
```

Si se corta la conexión, repetí **exactamente el mismo comando** (misma key): no se acredita dos veces.

### 3. Consultar el saldo

```bash
curl -s "$BASE/v1/accounts/$USER_ID/balance" -H "X-API-Key: $API_KEY"
```

```json
{ "externalUserId": "cli-10045", "balance": 1270, "updatedAt": "2026-10-07T18:42:10Z" }
```

### 4. Debitar puntos

```bash
curl -s -X POST "$BASE/v1/accounts/$USER_ID/debits" \
  -H "X-API-Key: $API_KEY" \
  -H "Idempotency-Key: 2b7d4e91-0c6a-4f3e-9b8d-7a1c5e2f0d34" \
  -H "Content-Type: application/json" \
  -d '{"points": 500, "reason": "Descuento aplicado en compra", "reference": "ORDER-A-7790"}'
```

### 5. Manejar saldo insuficiente

```bash
curl -s -X POST "$BASE/v1/accounts/$USER_ID/debits" \
  -H "X-API-Key: $API_KEY" \
  -H "Idempotency-Key: 5e8a1f20-7b3c-4d9e-a6f1-2c0b9d8e7a65" \
  -H "Content-Type: application/json" \
  -H "Prefer: code=422" \
  -d '{"points": 5000, "reason": "Descuento aplicado en compra", "reference": "ORDER-A-7795"}'
```

Respuesta `422` con `"code": "INSUFFICIENT_BALANCE"`. El header `Prefer` solo hace falta en el mock; en producción el 422 aparece cuando el saldo realmente no alcanza.

### 6. Canjear un beneficio

```bash
curl -s -X POST "$BASE/v1/accounts/$USER_ID/redemptions" \
  -H "X-API-Key: $API_KEY" \
  -H "Idempotency-Key: a4c9e2f7-1b8d-4a6e-9c3f-5d2b7e0a1f86" \
  -H "Content-Type: application/json" \
  -d '{"benefitId": "0e5f6a7b-8c9d-4e1f-a2b3-c4d5e6f70812", "reference": "CANJE-POS-118"}'
```

### 7. Ver los movimientos

```bash
curl -s "$BASE/v1/accounts/$USER_ID/movements?page=1&size=20" -H "X-API-Key: $API_KEY"
```

> En PowerShell, reemplazá `\` por `` ` `` al final de cada línea y usá `curl.exe` en lugar de `curl`.

## Validar el contrato

El contrato se valida con **Spectral** usando las reglas de [.spectral.yaml](.spectral.yaml) (OpenAPI válido, ejemplos válidos contra los esquemas, `Idempotency-Key` en todo POST, errores en `problem+json`, `Retry-After` en 429/503, rutas versionadas).

```bash
docker run --rm -v "$PWD:/work" -w /work stoplight/spectral:6.14.3 \
  lint docs/contracts/benefits-api.v1.yaml --ruleset docs/contracts/.spectral.yaml --fail-severity=warn
```

Con `make`: `make contract-lint`. La CI corre el mismo comando en cada PR que toque `docs/contracts/`.

## Política de versionado

El contrato sigue **Semantic Versioning** (`MAYOR.MENOR.PARCHE`, en `info.version`). La versión **mayor** va en la ruta (`/v1/...`).

| Tipo de cambio | Ejemplos | Versión | Ruta |
|---|---|---|---|
| **Compatible** | Nuevo endpoint; nuevo campo **opcional** en el request; nuevo campo en la respuesta; nuevo `code` de error; nuevo valor de `type` en movimientos | MENOR (1.1.0) | Sigue `/v1` |
| **Corrección** | Descripciones, ejemplos, errores tipográficos | PARCHE (1.0.1) | Sigue `/v1` |
| **Incompatible** | Quitar o renombrar un campo o endpoint; volver obligatorio un campo; cambiar tipos, formatos o semántica; achicar límites | MAYOR (2.0.0) | Nueva ruta `/v2` |

Lo que esperamos de tu cliente para que los cambios compatibles no te rompan:

- **Ignorá campos desconocidos** en las respuestas.
- **Tratá valores de enum desconocidos** (por ejemplo, un nuevo `type` de movimiento) sin fallar.
- Manejá códigos de error nuevos por su status HTTP.

**Deprecación:** cuando salga `/v2`, `/v1` se mantiene **al menos 60 días** (o hasta el fin de la cursada, lo que ocurra primero). Durante ese período las respuestas de `/v1` incluyen los headers `Deprecation: true` y `Sunset: <fecha>`, y el cambio se anuncia en este CHANGELOG y a los contactos técnicos de cada partner.

## CHANGELOG del contrato

### v1.0.0 — 2026-10-07

- Versión inicial publicada.
- Operaciones: acreditar (`credits`), debitar (`debits`), saldo (`balance`), movimientos paginados (`movements`), catálogo (`benefits`) y canjes (`redemptions`).
- Autenticación por `X-API-Key`; idempotencia obligatoria en escrituras con retención de 30 días.
- Errores RFC 7807 con `code` estable: `INVALID_REQUEST`, `UNAUTHORIZED`, `FORBIDDEN`, `ACCOUNT_NOT_FOUND`, `BENEFIT_NOT_FOUND`, `IDEMPOTENCY_CONFLICT`, `IDEMPOTENCY_IN_PROGRESS`, `INSUFFICIENT_BALANCE`, `BENEFIT_NOT_AVAILABLE`, `RATE_LIMITED`, `INTERNAL`, `UNAVAILABLE`.
- Mock ejecutable con Prism.
