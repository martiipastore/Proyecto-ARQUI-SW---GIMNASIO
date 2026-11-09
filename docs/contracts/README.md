# Club de Beneficios — Guía para integrar la API de fidelización

> Capacidad ofrecida al grupo consumidor: registro externo, acreditación, saldo e
> historial de movimientos. El registro requiere habilitar identidad y correo.
> La API pública HTTPS está disponible en `https://metalfitness-club.vercel.app/partner-api/v2`.
> Detalles de activación y configuración: [Registro externo](../CLUB_REGISTRO_EXTERNO.md).

Esta guía es para equipos de **otros sistemas** que quieran registrar a sus usuarios en el Club, acreditarles puntos por acciones de su sistema y consultar el saldo y los movimientos.

| | |
|---|---|
| **Contrato operativo** | [benefits-api.v2.yaml](benefits-api.v2.yaml) (OpenAPI 3.1) |
| **Versión vigente** | **v2.0.0** (`/v2`) |
| **Contrato anterior** | [benefits-api.v1.yaml](benefits-api.v1.yaml), histórico; contiene operaciones que no se implementaron y no debe usarse para una integración nueva |
| **Mock local** | `http://localhost:4010` (ver [Probar con el mock](#probar-con-el-mock)) |
| **Producción** | `https://metalfitness-club.vercel.app/partner-api/v2` |
| **Formato** | JSON (UTF-8); errores en `application/problem+json` (RFC 7807) |

---

## Qué ofrece

| Operación | Método y ruta | Para qué |
|---|---|---|
| Acreditar puntos | `POST /v2/accounts/{externalUserId}/credits` | Premiar algo que hizo el usuario en tu sistema |
| Consultar saldo | `GET /v2/accounts/{externalUserId}/balance` | Mostrar los puntos actuales de la cuenta |
| Listar movimientos | `GET /v2/accounts/{externalUserId}/movements?page=1&size=20` | Mostrar las acreditaciones hechas por tu integración |
| Registrar o vincular usuario | `POST /v2/external-register` | Crear o vincular una cuenta, con puntos opcionales |
| Consultar registro | `GET /v2/external-registrations/{eventId}` | Saber si terminó el alta y la acreditación inicial |

### Conceptos clave

- **Cuenta = (partner, `externalUserId`).** El `externalUserId` es **tu** identificador del usuario (1 a 64 caracteres: letras, números y `. _ : @ -`). No necesitás conocer IDs internos del gimnasio.
- **Las cuentas se vinculan antes de usarse.** Una persona puede tener cuenta sin membresía. Una integración con permiso de registro puede solicitar alta/vinculación por `POST /v2/external-register`; una cuenta previa exige prueba de control del correo. Si operás sobre un usuario no vinculado a vos recibís `404 ACCOUNT_NOT_FOUND`. En el despliegue, identidad y registro deben estar habilitados.
- **Saldo compartido, historial propio.** El saldo es el total de la cuenta (incluye, por ejemplo, puntos que el alumno ganó por asistir a clases). En `movements` ves **solo los movimientos que hiciste vos**.
- **Los movimientos son inmutables.** No hay endpoint público para borrar ni editar una acreditación.
- **El saldo nunca es negativo.** Esta capacidad solo suma puntos; no ofrece débitos ni canjes.

## Cómo obtener una API key

1. Pedí el alta como partner a cualquier integrante del equipo (ver [Equipo](../../README.md#equipo)) o abriendo un *issue* en este repositorio con el título `Alta de partner: <nombre de tu sistema>`. Indicá un contacto técnico.
2. El administrador del gimnasio crea el partner y te entrega la **API key una sola vez**. Guardala como secreto (variable de entorno o gestor de secretos); nunca en el código ni en el frontend.
3. Coordiná permisos y límites. Para vincular usuarios, usá el registro externo. Las integraciones nuevas deben enviar `eventId` en cada crédito.
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

Las escrituras (`external-register` y `credits`) **exigen** el header `Idempotency-Key`. Así podés reintentar ante un timeout sin registrar o acreditar dos veces. `eventId` identifica el hecho de negocio y también impide repetir la acreditación con otra key.

| Caso | Resultado |
|---|---|
| Key nueva | Se procesa la operación |
| Misma key + **mismo body** | Se devuelve **la respuesta original** (mismo status y body) con `Idempotent-Replayed: true`. No se aplica de nuevo. |
| Misma key + **body distinto** | `409 IDEMPOTENCY_CONFLICT` |
| Misma key mientras la primera todavía se procesa | `409 IDEMPOTENCY_IN_PROGRESS` → reintentá en unos segundos con la misma key |
| Reintento de un crédito rechazado y persistido | Se devuelve **el mismo rechazo**. Para una operación distinta, usá otra key. |

Reglas prácticas:

- Generá **una key por operación lógica** (recomendado: UUID v4) y guardala junto a tu registro de origen (la compra, el turno). Entre 8 y 128 caracteres: letras, números y `. _ : -`.
- Las keys de créditos son únicas **por partner** y se conservan **al menos 30 días**. Conservá también el `eventId`: su deduplicación no caduca con la key HTTP.
- Las consultas (`GET`) son seguras de reintentar y no usan key.

## Errores

Todos los errores tienen este formato:

```json
{
  "type": "https://gimnasio.example.com/problems/account-not-found",
  "title": "Cuenta no encontrada",
  "status": 404,
  "detail": "La cuenta no está vinculada a este partner.",
  "instance": "/partner-api/v2/accounts/cli-10045/balance",
  "code": "ACCOUNT_NOT_FOUND",
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
| `IDEMPOTENCY_CONFLICT` | 409 | Reusaste una key con otro body. | **No.** Usá una key nueva si es otra operación. |
| `IDEMPOTENCY_IN_PROGRESS` | 409 | La operación original con esa key todavía está en curso. | **Sí**, con la misma key, tras 1–2 s. |
| `RATE_LIMITED` | 429 | Superaste el límite de uso. | **Sí**, después de `Retry-After` segundos. |
| `INTERNAL` | 500 | Error inesperado de nuestro lado. | **Sí**, con backoff exponencial y la **misma key**. |
| `UNAVAILABLE` | 502 / 503 | Servicio caído o en mantenimiento. | **Sí**, después de `Retry-After`, con la **misma key**. |
| `TIMEOUT` | 504 | El gateway agotó la espera; el resultado de la operación es indeterminado. | **Sí**, con backoff y la **misma key**. |
| *(timeout / error de red)* | — | No sabés si se aplicó. | **Sí**, con la **misma key**: si ya se aplicó, recibís la respuesta original. |

**Backoff recomendado:** 1 s, 2 s, 4 s, 8 s (máximo 5 intentos), con un poco de aleatoriedad. Siempre respetá `Retry-After` cuando viene.

## Límites de uso

| Límite | Valor |
|---|---|
| Solicitudes por API key | 120 por minuto por defecto, configurable por integración (ventana de minuto calendario) |
| Puntos por operación | 1 a 1.000.000 (entero) |
| `reason` | 3 a 200 caracteres |
| `reference` | 1 a 100 caracteres |
| Página de movimientos (`size`) | 1 a 100 (por defecto 20) |
| Retención de `Idempotency-Key` | 30 días |
| Timeout público del gateway | 5 s |
| Timeout de Benefits para estas solicitudes | 4 s |

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
| `Prefer: code=404` | Devuelve la respuesta 404 de la operación |
| `Prefer: example=idempotencyInProgress` | Devuelve el ejemplo con ese nombre |
| `Prefer: dynamic=true` | Genera datos aleatorios válidos según el esquema |

## Ejemplos paso a paso

Las variables de abajo apuntan al mock. Para producción, usá `BASE=https://metalfitness-club.vercel.app/partner-api` y la API key asignada a tu integración. Enviá la API key solo desde el backend de tu sistema; nunca desde código del navegador.

```bash
BASE=http://localhost:4010
API_KEY=demo-partner-key
USER_ID=cli-10045
```

### 1. Registrar o vincular a una persona

```bash
curl -s -X POST "$BASE/v2/external-register" \
  -H "X-API-Key: $API_KEY" \
  -H "Idempotency-Key: registro-ORDER-A-7781" \
  -H "Content-Type: application/json" \
  -d '{"name":"Persona de ejemplo","email":"persona@example.test","externalUserId":"cli-10045","eventId":"alta-ORDER-A-7781","points":0}'
```

El servicio real devuelve `202` con `registrationId`, `eventId`, `externalUserId` y `status: "accepted"`. Si la cuenta ya existía, su titular debe confirmar el enlace recibido por correo. El mock devuelve un ejemplo fijo y no envía correos. Conservá el `externalUserId` devuelto si no lo enviaste.

### 2. Consultar el resultado del registro

```bash
curl -s "$BASE/v2/external-registrations/alta-ORDER-A-7781" -H "X-API-Key: $API_KEY"
```

En el servicio real, esperá `status: "completed"` antes de consultar el saldo o acreditar un evento nuevo. Respetá `Retry-After` si recibís 429. El mock no cambia de estado.

### 3. Acreditar puntos por una compra

```bash
curl -s -X POST "$BASE/v2/accounts/$USER_ID/credits" \
  -H "X-API-Key: $API_KEY" \
  -H "Idempotency-Key: 9f1b6c2e-3a4d-4e5f-8a9b-0c1d2e3f4a5b" \
  -H "Content-Type: application/json" \
  -d '{"eventId":"ORDER-A-7781","points":150,"reason":"Compra en tienda online","reference":"ORDER-A-7781"}'
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

Si se corta la conexión, repetí **exactamente el mismo comando**, con la misma key y el mismo `eventId`: no se acredita dos veces.

### 4. Consultar el saldo

```bash
curl -s "$BASE/v2/accounts/$USER_ID/balance" -H "X-API-Key: $API_KEY"
```

```json
{ "externalUserId": "cli-10045", "balance": 1270, "updatedAt": "2026-10-07T18:42:10Z" }
```

### 5. Ver los movimientos

```bash
curl -s "$BASE/v2/accounts/$USER_ID/movements?page=1&size=20" -H "X-API-Key: $API_KEY"
```

> En PowerShell, reemplazá `\` por `` ` `` al final de cada línea y usá `curl.exe` en lugar de `curl`.

## Validar el contrato

El contrato se valida con **Spectral** usando las reglas de [.spectral.yaml](.spectral.yaml) (OpenAPI válido, ejemplos válidos contra los esquemas, `Idempotency-Key` en todo POST, errores en `problem+json`, `Retry-After` en 429/503, rutas versionadas).

```bash
docker run --rm -v "$PWD:/work" -w /work stoplight/spectral:6.14.3 \
  lint docs/contracts/benefits-api.v2.yaml --ruleset docs/contracts/.spectral.yaml --fail-severity=warn
```

Con `make`: `make contract-lint`. La CI corre el mismo comando en cada PR que toque `docs/contracts/`.

La [prueba E2E local de v2](../evidence/club/cb04-v2-e2e.json) aprobó 14 escenarios
con Gateway, Benefits, Members, PostgreSQL y Mailpit reales el 08/11/2026.
Incluye registro, crédito, aislamiento, fallos SMTP y conservación de puntos al
cambiar la membresía. El 08/11/2026 se comprobó también el despliegue público en
Vercel con una cuenta de prueba: registro completado, correo y activación,
acreditación de 25 puntos, saldo actualizado, historial e idempotencia.

## Política de versionado

El contrato sigue **Semantic Versioning** (`MAYOR.MENOR.PARCHE`, en `info.version`). La versión **mayor** va en la ruta (`/v2/...`).

| Tipo de cambio | Ejemplos | Versión | Ruta |
|---|---|---|---|
| **Compatible** | Nuevo endpoint o campo opcional; nuevo campo en la respuesta; nuevo código de error | MENOR (2.1.0) | Sigue `/v2` |
| **Corrección** | Aclarar descripciones o ejemplos sin cambiar el comportamiento | PARCHE (2.0.1) | Sigue `/v2` |
| **Incompatible** | Quitar o renombrar un campo o endpoint; volver obligatorio un campo; cambiar tipos, formatos o semántica; achicar límites | MAYOR (3.0.0) | Nueva ruta `/v3` |

Lo que esperamos de tu cliente para que los cambios compatibles no te rompan:

- **Ignorá campos desconocidos** en las respuestas.
- **Tratá valores de enum desconocidos** sin fallar.
- Manejá códigos de error nuevos por su status HTTP.

**Versión anterior:** `/v1` permanece en el código y el contrato 1.4.0 se conserva como historial. Ese contrato incluía rutas nunca implementadas; por eso no debe usarse para una integración nueva. La v2 es la primera versión ajustada al alcance que se ofrecerá públicamente. Una futura retirada de una versión operativa requerirá aviso previo, período de coexistencia y fecha de fin documentada.

## CHANGELOG del contrato

### v2.0.0 — 2026-11-08

- Contrato operativo para registro externo, consulta de su recibo, acreditación,
  saldo y movimientos. La ruta pública cambia a `/partner-api/v2`; el gateway
  adapta esa ruta a los handlers actuales de Benefits.
- Débitos, catálogo y canjes se retiran del contrato vigente por decisión del
  equipo. Se conserva el contrato v1 como historial de lo anunciado antes.
- Los ejemplos de créditos incluyen `eventId`, requerido por defecto para
  integraciones nuevas. La cuota documentada se ajusta al código: 120/minuto
  por defecto, configurable por integración.
- La publicación HTTPS y las pruebas desde una red externa siguen pendientes.

### v1.4.0 — 2026-11-06

- Política `requiere_evento` validada en backend: integraciones nuevas requieren
  `eventId` en créditos; las existentes conservan su comportamiento anterior.
  No se cambia la ruta ni el esquema global de campos requeridos para clientes previos.
- Evento ausente cuando es obligatorio: 400 `INVALID_REQUEST`, sin acreditar.
  Un mismo evento con otra key conserva movimiento y saldo originales; otro
  usuario o contenido provoca 409. Permisos/cuotas/políticas siguen vigentes en replay.
- Fechas de créditos en UTC con precisión de microsegundos desde su primera
  respuesta para conservar su representación al recuperar eventos de PostgreSQL.
- La cuota 429 se verifica con `Retry-After`; los rechazos de política previos a
  tomar la key no son resultados de una acreditación confirmada ni reservan esa key.

### v1.3.0 — 2026-11-06

- `POST /v1/external-register`: registro/vinculación con identidad verificada,
  puntos opcionales y recibo asíncrono persistente.
- `GET /v1/external-registrations/{eventId}`: consulta privada de procesamiento.
- Permiso independiente de registro, desactivado por defecto. Cuota por integración
  configurable; agotarla devuelve 429 `RATE_LIMITED` con `Retry-After`.
- La plataforma se identifica por su credencial. El mismo evento se deduplica
  también entre el registro y el endpoint de acreditaciones.

### v1.2.0 — 2026-11-06

- `credits` acepta `eventId` opcional (1–128 caracteres), único por integración,
  para evitar una segunda acreditación aunque cambie `Idempotency-Key`.
- Permisos separados para consulta y puntos, límite por operación y presupuesto
  diario configurables en backend. Permiso retirado: 403; tope excedido: 400.
- Cuerpo de acreditación limitado a 16 KiB. Se conservan solicitudes anteriores
  sin `eventId`; para nuevas integraciones enviarlo siempre.
- Los endpoints previstos de registro, débito y canje no se anuncian como ejecutables.

### v1.1.1 — 2026-10-16

- Se aclara Balance.updatedAt: fecha del último movimiento aplicado o de creación de la cuenta si todavía no tiene movimientos (ADR-008 v3, D-53).
- El flujo mínimo real incluye créditos, saldo y movimientos; los demás endpoints conservan su contrato y mock, pero su implementación está pendiente. Ver docs/entrega-2/ETAPA_3_API_BENEFITS.md.

### v1.1.0 — 2026-10-16

- Se documentan 502 UNAVAILABLE y 504 TIMEOUT en todas las operaciones, con formato de errores de partners y correlación. 502 incluye Retry-After.
- Reintentar con la misma Idempotency-Key; 504 no implica rollback.
- Se mantienen rutas /v1, solicitudes, respuestas de éxito y timeout público de 5 s.
- Diseño aprobado en ADR-008 v2. La traducción de errores del gateway se implementó posteriormente en la etapa 3 y se validó en la etapa 6; el contrato por sí solo no acredita esa implementación.

### v1.0.0 — 2026-10-07

- Versión inicial publicada.
- Operaciones: acreditar (`credits`), debitar (`debits`), saldo (`balance`), movimientos paginados (`movements`), catálogo (`benefits`) y canjes (`redemptions`).
- Autenticación por `X-API-Key`; idempotencia obligatoria en escrituras con retención de 30 días.
- Errores RFC 7807 con `code` estable: `INVALID_REQUEST`, `UNAUTHORIZED`, `FORBIDDEN`, `ACCOUNT_NOT_FOUND`, `BENEFIT_NOT_FOUND`, `IDEMPOTENCY_CONFLICT`, `IDEMPOTENCY_IN_PROGRESS`, `INSUFFICIENT_BALANCE`, `BENEFIT_NOT_AVAILABLE`, `RATE_LIMITED`, `INTERNAL`, `UNAVAILABLE`.
- Mock ejecutable con Prism.
