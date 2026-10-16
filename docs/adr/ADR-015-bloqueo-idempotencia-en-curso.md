# ADR-015 — Intentar un bloqueo transaccional por clave de idempotencia

| Campo | Valor |
|---|---|
| **Estado** | Aceptado el 16/10/2026; implementado en etapa 3 |
| **Fecha** | 2026-10-16 |
| **Decisión del TP** | D4 — detalle de concurrencia; D8 — respuesta en curso |
| **Relacionado con** | RN-27; CL-08/08b/08c; ADR-004 v2; D-54 del registro |

## Contexto

La unicidad de `(partner_id, clave)` ya evita duplicados, pero una inserción concurrente puede esperar a la primera transacción. El contrato también define 409 IDEMPOTENCY_IN_PROGRESS cuando la primera ejecución sigue en curso.

## Alternativas consideradas

| Alternativa | Ventaja | Límite |
|---|---|---|
| Esperar solo con unicidad SQL | No duplica movimientos | No distingue de inmediato una operación en curso |
| Registro de locks en memoria | Acceso simple | No coordina procesos ni sobrevive a la caída de una instancia |
| Intento de bloqueo transaccional en PostgreSQL | Coordina procesos y se libera al terminar la transacción | Requiere un mecanismo específico del motor |

## Decisión y justificación

El equipo aprobó `pg_try_advisory_xact_lock` por la identidad partner/clave. Si no se obtiene, devolver el error de dominio de operación en curso, traducido a 409 por HTTP. Mantener unicidad SQL, huella, resultado durable y bloqueo de cuenta ya aprobados. No se agrega un servicio, framework ni almacén de locks.

Fundamento de clase: transacciones locales e idempotencia (pp. 27–28, 103–105 y 113–115). La función concreta de PostgreSQL no se enseña en el apunte: es un detalle adicional aprobado para implementar el comportamiento contractual sin esperar otra transacción. El lock vive hasta commit/rollback; no decide el saldo ni reemplaza la restricción única.

La clave de lock se calcula con un hash de 64 bits. Una colisión podría provocar un 409 temporal entre operaciones independientes, pero no permite duplicar un movimiento. Se conserva la clave completa para comparar identidad y resultado; el consumidor reintenta con la misma clave. No se promete equidad entre solicitudes concurrentes.

## Cómo se verifica

Mantener el bloqueo desde otra transacción y comprobar 409 IDEMPOTENCY_IN_PROGRESS sin movimiento nuevo; liberar y comprobar que la operación pueda ejecutarse. Las pruebas concurrentes admiten éxito/replay o en curso; después del commit, la repetición recupera el original y un payload distinto produce conflicto. Ver ETAPA_3_API_BENEFITS.md.
