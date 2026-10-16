# ADR-004 v2 — Orden, atomicidad y suma íntegra del ledger

| Campo | Valor |
|---|---|
| **Estado** | Aceptado como aclaración el 16/10/2026; implementación pendiente |
| **Fecha** | 2026-10-16 |
| **Decisión del TP** | D4 |
| **Reemplaza** | [ADR-004](ADR-004-orden-movimientos-puntos.md) |
| **Autores** | Equipo del TPI |
| **Relacionado con** | SPEC RN-20, RN-22, RN-23, RN-38 y RN-42; [ADR-003](ADR-003-persistencia.md); [ADR-005 v2](ADR-005-v2-comunicacion.md) |

## Motivo de la revisión y fundamento

El equipo aprobó alinear el glosario y la verificación del saldo con RN-22, CA-11.1 y CA-19.2 el 16/10/2026. La expresión «suma de movimientos confirmados» era ambigua cuando el original pasa a REVERTIDO y existe un compensatorio. Esta revisión conserva íntegramente el orden y mecanismo transaccional del ADR original y aclara qué se suma.

Fundamento teórico: transacciones locales e invariantes dentro del servicio (pp. 27–28 del teórico) y agregado como límite de consistencia (p. 120). La fórmula concreta del saldo proviene del SPEC, no se atribuye al apunte. No se introduce Event Sourcing ni otra arquitectura por esta aclaración.

## Contexto

Benefits recibe movimientos desde HTTP y desde eventos que pueden demorarse, reintentarse o llegar en distinto orden. La acreditación por asistencia suma 500 puntos. La penalización por inasistencia descuenta `min(100, saldo_actual)` y nunca permite saldo negativo.

Estas operaciones no son conmutativas. Desde saldo 0:

- penalización y luego acreditación deja saldo 500;
- acreditación y luego penalización deja saldo 400.

La entrega idempotente evita duplicados, pero no define cuál operación se aplica primero. RabbitMQ tampoco garantiza un orden global entre productores, réplicas y operaciones HTTP.

## Alternativas consideradas

### Alternativa A — Suponer que los eventos llegan en orden

Descartada. Reintentos, DLQ, múltiples instancias y canales distintos pueden alterar el orden de llegada.

### Alternativa B — Reordenar siempre por la fecha del hecho

Descartada. Un evento antiguo puede llegar después de movimientos ya confirmados. Insertarlo retrospectivamente obligaría a recalcular penalizaciones y saldos o modificar movimientos inmutables. Tampoco existe una señal global que garantice que ya llegaron todos los hechos anteriores.

### Alternativa C — Mantener una deuda de penalización

Descartada para esta versión. Haría conmutativas algunas operaciones, pero cambia la regla vigente: una penalización sin saldo pasaría a descontar acreditaciones futuras.

### Alternativa D — Serializar por cuenta según el orden de aplicación (elegida)

Cada operación toma un bloqueo sobre la cuenta, calcula su efecto con el saldo confirmado, asigna una secuencia creciente y realiza todos sus cambios en una transacción local.

## Decisión

`CuentaBeneficios` mantiene `saldo` y `ultima_secuencia`. Toda operación que pueda modificarla sigue este orden:

1. Inicia una transacción.
2. Resuelve idempotencia o deduplicación por la clave/origen correspondiente.
3. Bloquea la fila de la cuenta con `SELECT ... FOR UPDATE`.
4. Valida la operación usando el saldo bloqueado.
5. Calcula el importe. Para una inasistencia: `importe = -min(100, saldo)`.
6. Incrementa `ultima_secuencia` e inserta el movimiento con esa secuencia.
7. Actualiza el saldo y registra la operación o el mensaje procesado en la misma transacción.
8. Confirma la transacción y recién entonces responde o realiza ACK.

Todos los caminos usan el mismo mecanismo: asistencia, inasistencia, crédito o débito de partner, canje y reversión. Por eso dos instancias de benefits no pueden modificar simultáneamente una cuenta.

### Saldo y movimientos revertidos

El saldo es la suma algebraica de todos los importes aplicados de la cuenta, incluidos los movimientos originales cuyo estado sea REVERTIDO y los compensatorios con su signo. REVERTIDO impide volver a revertir el original; no retira su importe de la historia ni modifica por sí mismo el saldo.

Ejemplo: crédito original +100 y compensatorio −100 dejan saldo 0. Excluir el original de la suma produciría −100 y contradice RN-22. Original, compensatorio, estado y saldo se modifican según las garantías transaccionales ya acordadas; no se permite borrar ni editar los importes históricos.

Alternativas: excluir originales REVERTIDOS se descarta por el resultado incorrecto; borrar el original se descarta por romper la auditoría e inmutabilidad. Se conserva original más compensatorio porque coincide con los criterios funcionales aprobados.

### Tiempos y orden

Cada movimiento conserva:

- `occurred_at`: cuándo ocurrió el hecho en el sistema de origen;
- `applied_at`: cuándo benefits confirmó su efecto;
- `account_sequence`: orden estricto de aplicación dentro de la cuenta.

`account_sequence` es el orden canónico para reconstruir el saldo. `occurred_at` sirve para explicar el origen y no reordena movimientos confirmados.

### Penalización con saldo cero

Se inserta un movimiento `PENALIZACION_INASISTENCIA` de 0 puntos con su secuencia y origen. Aunque no cambie el saldo, deja evidencia de que la ausencia fue procesada e impide que una reentrega descuente puntos acreditados posteriormente.

### Operaciones concurrentes

Si una acreditación y una penalización compiten, PostgreSQL decide cuál obtiene primero el bloqueo. Ambos resultados son válidos conforme a RN-42:

| Orden aplicado | Movimientos | Saldo final desde 0 |
|---|---|---:|
| Penalización → acreditación | 0, +500 | 500 |
| Acreditación → penalización | +500, -100 | 400 |

El sistema no promete prioridad entre hechos concurrentes. Sí garantiza una única secuencia observable, saldo no negativo, movimientos inmutables y ausencia de efectos duplicados.

## Consecuencias

### Positivas

- El ledger siempre explica el saldo mediante un orden único por cuenta.
- No se depende del orden de RabbitMQ ni del reloj de otros servicios.
- Las invariantes de saldo e idempotencia se resuelven en una transacción local.
- Un evento tardío no obliga a reescribir el historial.

### Negativas

- El resultado de operaciones concurrentes puede ser 400 o 500 según el orden de aplicación.
- Una cuenta muy activa serializa sus escrituras sobre una misma fila.
- Los listados deben distinguir la fecha del hecho de la fecha y secuencia de aplicación.
- Las penalizaciones de 0 deben representarse claramente para no confundir al usuario.

## Cómo se verificará

- Dos operaciones concurrentes sobre una cuenta reciben secuencias diferentes y consecutivas.
- Con saldo 0, penalización seguida de acreditación produce movimientos 0 y +500, saldo 500.
- Con saldo 0, acreditación seguida de penalización produce +500 y -100, saldo 400.
- Reentregar cualquiera de los eventos no agrega movimientos ni consume otra secuencia.
- Débitos, canjes y reversiones concurrentes nunca producen saldo negativo.
- La suma de todos los importes aplicados, incluidos originales REVERTIDOS y compensatorios, coincide con `cuentas.saldo` después de cada prueba.
- Crédito +100 y reversión −100 dejan saldo 0; otra reversión del mismo original se rechaza sin movimientos nuevos.
