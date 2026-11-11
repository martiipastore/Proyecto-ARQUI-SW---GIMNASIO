# Cuponera interna del Club de Beneficios (D-94)

**Estado:** implementada y verificada localmente el 11/11/2026. Tests unitarios, de
integración con PostgreSQL real (testcontainers) y una prueba de punta a punta por el
gateway con 24 chequeos, todo aprobado. Falta la revisión de otro integrante. No se
publicó en Vercel.

Aprobada por el equipo el 11/11/2026 (D-94). Reemplaza parcialmente D-76 **solo para la
aplicación del gimnasio**: los alumnos canjean puntos por beneficios de un catálogo. La
[API de partners v2](../contracts/README.md), ADR-008 v4 y el contrato **no cambian**:
los otros grupos siguen sin débitos ni canjes.

Historias y reglas: HU-09 (CA-09.3 a 09.5), HU-20 (CA-20.1 a 20.6), RN-15, RN-22,
RN-23, RN-24 y RN-25; casos límite CL-07, CL-07b y CL-18. Contrato:
[benefits.v1](../api/benefits.v1.md#cuponera-interna-d-94).

## 1. Decisiones

| Tema | Decisión | Origen |
|---|---|---|
| Stock | **Sin stock**: un beneficio activo y vigente se canjea sin límite. | SPEC RN-24, CA-09.5 |
| Vigencia | Días calendario de Argentina; `vigente_hasta` es inclusivo y opcional. | SPEC (glosario "Beneficio") |
| Cupón | Cada canje entrega un código `XXXX-XXXX` (sin `0`, `1`, `I`, `L`, `O`) que recepción busca y marca **USADO**. | Pedido del equipo; CA-20.5 y CA-20.6 nuevos |
| Estados | `CONFIRMADO → USADO` o `CONFIRMADO → ANULADO`. Un USADO no se anula; un ANULADO no se usa. | RN-25 y D-94 |
| Anulación | Devuelve los puntos con un compensatorio `REVERSION` en el mismo ledger. La reversión genérica de la etapa 7 no alcanza al débito de un canje. | RN-25 ("anular un canje revierte su débito") |
| Quién canjea | Solo el rol ALUMNO, sobre su propia cuenta. Sin cuenta de puntos: `422 SALDO_INSUFICIENTE`. | HU-20 |
| Exportación | No se expone por `/partner-api`. | D-76, ADR-008 v4 |

## 2. Qué se agregó

| Endpoint (gateway `/api/v1`) | Quién | Qué hace |
|---|---|---|
| `GET /benefits/catalog` | Alumno, administrador | Beneficios canjeables hoy. |
| `POST /benefits/redemptions` | Alumno | Canjea con `Idempotency-Key` obligatoria; débito, cupón y saldo en una transacción. |
| `GET /benefits/me/redemptions` | Alumno | Cupones propios con su código y estado. |
| `GET /benefits/rewards` | Administrador | Catálogo completo, con inactivos y cantidad de canjes. |
| `POST /benefits/rewards`, `PUT /benefits/rewards/{id}` | Administrador | Alta y modificación (nombre, descripción, costo, vigencia, activo). |
| `GET /benefits/redemptions?codigo=&estado=` | Administrador | Busca cupones; acepta el código en minúsculas o sin guion. |
| `POST /benefits/redemptions/{id}/use` | Administrador | Marca el cupón como USADO. |
| `POST /benefits/redemptions/{id}/cancel` | Administrador | Anula con `motivo` y devuelve los puntos. |

Pantallas:

- `#/app/cuponera` (menú **Cuponera** del alumno): saldo, catálogo con el botón
  **Canjear** (o "Te faltan N pts"), confirmación accesible y el cupón recién canjeado
  con una animación de sello solo de CSS, estática con `prefers-reduced-motion` (D-89).
  Debajo, **Mis cupones**. El inicio del alumno suma un enlace "Canjear puntos".
- `#/admin/cuponera` (menú **Cuponera** del administrador): validar un cupón por
  código, catálogo con alta y edición, y últimos canjes filtrables por estado.
- Historial de puntos: los débitos aparecen con origen **canje** y su código. En la
  ficha de persona, un débito de canje no ofrece **Revertir**: remite a la cuponera.

## 3. Arquitectura

Respeta el patrón hexagonal de benefits-service (ADR-002 v2):

| Capa | Archivo | Contenido |
|---|---|---|
| Dominio | `internal/domain/canje.go` | `Beneficio`, `CanjeBeneficio`, `Canjear`, `Usar`, `AnularCanje`, código y día de negocio. Solo biblioteca estándar. |
| Puertos | `internal/ports/canje.go` | `Cuponera` (salida) y `CodigoGenerator`. |
| Aplicación | `internal/application/cuponera.go` | Autorización por rol, huella idempotente y reintento si el código choca. |
| Adaptadores | `adapters/postgres/cuponera.go`, `adapters/http/cuponera.go`, `adapters/system` | Transacciones con bloqueo de cuenta, HTTP RFC 7807 y `crypto/rand`. |
| Gateway | `gateway/internal/proxy/access.go` | `benefitsAllowed`: el alumno llega solo a catálogo, canje y sus cupones. |

Migración `009_cuponera.sql`. La base garantiza, además del código (regla 8):

- saldo ≥ 0 con el chequeo y el trigger diferido del ledger que ya existían;
- código del cupón único e `Idempotency-Key` única por alumno;
- movimiento `CANJE` con importe entre −1.000.000 y −1;
- un débito `CANJE` sin su cupón (o con otro importe o cuenta) no se confirma, y
  tampoco un cupón sin su débito (trigger diferido `verificar_canje`);
- la reversión del débito de un canje solo se confirma si el cupón pasa a ANULADO
  con ese compensatorio;
- el cupón no se borra, solo admite las transiciones de §1 y sus datos son inmutables;
- auditoría de alta y cambio de beneficios, uso y anulación en `auditoria_club`.

El beneficio se lee con `FOR SHARE` dentro de la transacción del canje, así que un
cambio de costo concurrente espera y débito y cupón registran el mismo valor (CL-18).

## 4. Verificación

| Prueba | Resultado |
|---|---|
| `go test ./...` y `go vet ./...` en benefits-service | Aprobado, incluida la migración 009 sobre base vacía y repetida. |
| Integración con PostgreSQL real (`TestLedger_PostgreSQLReal/Cuponera`) | 6 escenarios aprobados: CA-20.1 a 20.4, CL-07b con 4 canjes concurrentes y saldo para uno, cuenta inexistente, CA-09.3/4/5, uso y anulación, e invariantes de la base frente a escrituras directas. |
| Regresión de etapas 6, 7 y 13 en el mismo test | Aprobada (Booking, MELT, Etapa 7, CB02). |
| Tests HTTP y de dominio | Aprobados: identidad del gateway, idempotencia, mapeo de errores y casos límite de vigencia. |
| Gateway (`TestBenefitsAllowed`) | Aprobado: matriz por rol de las 10 rutas. |
| Frontend | `tsc --noEmit` y `npm run build` aprobados. |
| Punta a punta por el gateway, stack local | 24 chequeos aprobados: alta del catálogo, canje, doble clic, saldo insuficiente, búsqueda por código, anulación idempotente, uso de un anulado, bloqueo de la reversión genérica y permisos de alumno, profesor y administrador. |

No se verificó visualmente en un navegador automatizado: las pantallas se probaron
con el build y el contrato, no con capturas.

## 5. Pendientes

- Revisión de otro integrante y prueba manual de las dos pantallas.
- Sincronizar el Word de decisiones (`scripts/sync-club-docs.ps1`), bloqueado desde la
  etapa 8 por una tabla de columnas incompatibles.
- Riesgo detectado al probar en Windows, que no depende de esta etapa: con
  `core.autocrlf=true`, Git puede reescribir las migraciones `.sql` con CRLF al cambiar
  de rama, y Benefits rechaza la migración por "modificada después de aplicarse". Se
  propone que el equipo decida entre fijar `*.sql text eol=lf` en `.gitattributes` o
  normalizar los finales de línea al calcular la huella.
