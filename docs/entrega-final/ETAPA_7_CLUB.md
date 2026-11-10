# Etapa 7 — Administración del Club de Beneficios

**Estado:** verificación técnica local cerrada el 10/11/2026. El E2E aprobó
**38 escenarios**: los 32 de las etapas 4 a 6 y 6 nuevos de esta etapa. Evidencia en
[club-e2e.json](../evidence/entrega-final/etapa7/club-e2e.json). Falta la revisión
de otro integrante. No se publicó en Vercel.

Alcance reducido por D-76: no hay catálogo, canjes ni débitos. La etapa cubre la
gestión de integraciones, la vista del administrador sobre la cuenta de una persona
y la reversión de movimientos.

Historias y reglas: HU-10 (CA-10.1 a 10.4), HU-11 (CA-11.1 a 11.3), HU-35 (CA-35.3)
y HU-CB-07; RN-22, RN-23, RN-25, RN-26 y CL-13. Contrato:
[benefits.v1](../api/benefits.v1.md). La [API de partners v2](../contracts/README.md)
no cambia.

## 1. Qué se agregó

| Endpoint (gateway `/api/v1`) | Qué hace |
|---|---|
| `GET /benefits/partners` | Lista las integraciones con su estado, permisos, límites y cantidad de cuentas vinculadas. Nunca devuelve la credencial. |
| `POST /benefits/partners` | Da de alta una integración ACTIVA (CA-10.1) y devuelve la API key **una sola vez**. |
| `PUT /benefits/partners/{id}` | Reemplaza nombre, estado, permisos y límites. Desactivarla corta el acceso (CA-10.2). |
| `POST /benefits/partners/{id}/rotate-key` | Emite una key nueva. La anterior deja de funcionar en la misma transacción. |
| `POST /benefits/partners/{id}/links` | Vincula la cuenta de una persona con el identificador externo del partner (CA-10.3). Si el identificador o la cuenta ya están vinculados de otra forma, responde 409 (CA-10.4). |
| `GET /benefits/accounts/{usuarioId}` | Saldo, vinculaciones y movimientos paginados de una persona, con estado y origen. |
| `POST /benefits/movements/{id}/revert` | Revierte un movimiento con motivo. Exige `Idempotency-Key` (HU-11). |

Pantallas:

- `#/admin/club` (menú **Club**): lista las integraciones y permite alta, edición,
  activación o desactivación y rotación. La key emitida se muestra una sola vez,
  con aviso y botón para copiarla.
- Ficha de persona `#/admin/personas/{id}`: nueva sección **Club de Beneficios**
  con saldo, vinculaciones, formulario para vincular, movimientos paginados y
  botón **Revertir** que pide el motivo. Se muestra para todos los roles, porque
  el Club no se restringe a alumnos (D-63/D-64).
- El historial propio del alumno (`#/app`, `#/cuenta`) muestra las reversiones como
  «corrección del administrador» y marca los originales como revertidos.

`scripts/provision-club-partner.ps1` y `cmd/partneradmin` siguen disponibles para
operar directo sobre la base, por ejemplo para dar de alta la primera integración
en Vercel antes de que exista un administrador.

## 2. Reversión e invariantes

1. La reversión es un movimiento compensatorio `REVERSION` en el **mismo ledger**,
   con la secuencia siguiente de la cuenta. El original no se modifica: el trigger
   de inmutabilidad de la migración 001 sigue vigente (RN-22).
2. El estado `REVERTIDO` se deriva: un original está revertido si existe un
   compensatorio con `revierte_id` apuntándolo. Así no hace falta actualizar
   movimientos. El saldo suma el original y su compensatorio (D-52).
3. La base garantiza las reglas además del código (regla 8 de CLAUDE.md):
   - un índice único sobre `revierte_id` permite una sola reversión por movimiento (CA-11.2);
   - el trigger `verificar_reversion` exige misma cuenta, importe exactamente
     contrario y que el original no sea otra reversión;
   - `cuentas.saldo >= 0` y `verificar_ledger` impiden el saldo negativo (CA-11.3, CL-13).
4. La cuenta se bloquea con `SELECT FOR UPDATE`, igual que los créditos de partners
   y los eventos de Booking: todas las escrituras de una cuenta quedan serializadas.
5. Revertir una asistencia no toca Booking. La reserva sigue `ASISTIDA` y una
   reentrega del evento no vuelve a acreditar, porque el recibo y el índice del
   hecho no cambian (CA-35.3).
6. Idempotencia: la clave es única por administrador. Con la misma clave y los
   mismos datos se devuelve el compensatorio original (`200`,
   `Idempotent-Replayed: true`). Con la misma clave y otro movimiento o motivo se
   responde `409 CONFLICTO_IDEMPOTENCIA`. Los rechazos de negocio no se guardan:
   se vuelven a evaluar si se reintenta.
7. Una penalización de −100 se revierte con +100. Una ausencia de 0 puntos no
   ofrece el botón de revertir en la pantalla; la API la acepta y genera un
   compensatorio de 0.

## 3. Seguridad y auditoría

- El gateway ya restringía estas rutas al rol ADMINISTRADOR. Benefits vuelve a
  autorizar en `application` (`ErrSinPermiso` → `403 SIN_PERMISO`) y exige la
  clave interna del gateway. Sin esa clave responde 401, aunque lleguen
  `X-User-Id`/`X-User-Role` (ADR-006, regla 10).
- Las API keys tienen 256 bits aleatorios y solo se guarda su hash SHA-256, igual
  que en la autenticación de partners. No se registran en logs ni en la auditoría.
- `auditoria_club` es inmutable y registra quién, qué y cuándo: alta, cambio de
  configuración, rotación, vinculación y reversión. Guarda la configuración
  resultante, nunca la credencial.
- Alta con valores seguros: registro externo desactivado, `eventId` obligatorio,
  1.000.000 puntos por operación, sin tope diario y 120 solicitudes por minuto
  (ADR-016). El alta no acepta `activo`: siempre queda ACTIVA (CA-10.1).

## 4. Persistencia

`007_administracion_club.sql` extiende el esquema sin modificar migraciones
históricas:

- `movimientos_puntos.revierte_id` con índice único parcial, y la restricción
  `importe_por_tipo` ampliada con `REVERSION` (−1.000.000 a +100, sin partner ni
  origen de Booking);
- trigger `verificar_reversion`;
- `reversiones_admin`: clave idempotente, huella, compensatorio y saldo resultante; inmutable;
- `auditoria_club`: inmutable;
- `partners.creada_en` y validación del nombre para filas nuevas (`NOT VALID`
  para no rechazar las existentes).

## 5. Decisiones de implementación y límites

- **El contrato de partners no cambia.** Un partner ve solo los movimientos que
  originó (RN-26). Si el administrador revierte un crédito suyo, el compensatorio
  no aparece en su listado y el original sigue `CONFIRMED`, porque el enum v2 no
  tiene otro estado. El saldo que consulta sí incluye la reversión. Mostrarlo
  requiere una versión nueva del contrato (ARCHITECTURE §7.1) y queda a decisión
  del equipo.
- **La vinculación manual requiere una cuenta de puntos existente.** La cuenta
  se crea con la primera asistencia o con un registro externo. La API no crea
  cuentas, para no inventar una regla de alta nueva.
- Desvincular no está en el SPEC y no se implementó.
- No hay automatización visual del navegador; se mantiene para el cierre de la etapa 11.

## 6. Verificación

Desde la raíz con Docker operativo:

```text
go run ./tests/e2e/identity/main.go -club
```

Incluye la regresión de las etapas 4 a 6 y agrega seis escenarios a través del
gateway: alta y key única, autorización (alumno 403, rol falsificado,
Benefits sin gateway 401), vinculación con crédito real del partner y conflicto,
rotación con revocación, desactivación y reactivación con auditoría, reversión
idempotente con estado y aislamiento del partner, y reversión de asistencia sin
tocar Booking. Para este último escenario el E2E se adapta al saldo que dejó la
etapa 6 (400 o 500, RN-42): con 500 revierte la asistencia (CA-35.3) y con 400
verifica `SALDO_INSUFICIENTE` (CA-11.3). Ambos casos están cubiertos de forma
determinística en los tests de integración.

`TestLedger_PostgreSQLReal/Etapa7` (PostgreSQL 16 real con rol sin privilegios)
cubre alta, hash, rotación, desactivación y auditoría sin credenciales;
vinculación CA-10.3/10.4; reversión con reintento, conflicto, doble reversión,
reversión de una reversión e inexistente; saldo insuficiente sin efectos;
reversión de asistencia con reentrega; ocho reversiones concurrentes con un solo
compensatorio; y rechazos de la base ante importes incompatibles, original de
otra cuenta y modificación del original. Hay además tests de dominio (CA-11.x y
límites de configuración), de aplicación (rol, key única con hash, huella
idempotente) y del adaptador HTTP (identidad del gateway, 403, valores por
defecto, PUT completo y catálogo de errores).

Resultados del 10/11/2026: `go vet` y `go test ./...` de Benefits aprobados con
Docker, sin tests omitidos; `tsc` y build de Vite aprobados; E2E de 38 escenarios
aprobado. Una primera corrida del E2E falló por tiempo agotado en la recuperación
SMTP de la etapa 5 (Docker recién iniciado). Ese escenario no cambió en esta etapa
y la segunda corrida completa aprobó todo.
