# ADR-010 v2 — Presupuestos y recuperación idempotente validados

| Campo | Valor |
|---|---|
| Estado | Aceptado; etapa 6 validada con ejercicio y evidencia exportada |
| Fecha | 2026-10-16 |
| Decisión del TP | D10 |
| Reemplaza | [ADR-010](ADR-010-resiliencia-benefits.md) |
| Registro | D-43, D-47 y D-56 |
| Relacionado | RN-22, RN-26, RN-27; CL-08/08b; ADR-005 v2, ADR-007, ADR-008 v3, ADR-011 v2, ADR-015 |

## Contexto y alternativas

La política inicial ya aprobada conserva 4 s para autenticación y operación de Benefits, dentro de 5 s del Gateway. Una respuesta perdida puede corresponder a una transacción confirmada; el error de transporte no determina rollback.

Se mantienen las alternativas y su elección: presupuesto y cancelación, recuperación explícita con la misma clave y sin reintentos de escrituras a nivel de aplicación en Gateway. No se agrega circuit breaker al flujo inicial. Las llamadas a otros servicios y al futuro proveedor conservan su alcance pendiente.

## Decisión que se mantiene

PostgreSQL es crítica. Movimiento, saldo y resultado idempotente se confirman en una transacción local; no se acredita en memoria ni se usa caché como respaldo. Se propaga cancelación al adaptador y se intenta limpiar la transacción con un contexto independiente de la cancelación. Esto no autoriza a asumir rollback ante un commit incierto: se resuelve reintentando la misma clave, que debe conservarse.

| Falla observada | Respuesta | Recuperación verificada |
|---|---|---|
| Benefits detenido | 502 UNAVAILABLE, Retry-After | Restaurar, reintentar misma clave y repetir sin duplicar |
| PostgreSQL no responde | 503 UNAVAILABLE, Retry-After | Restaurar base, comprobar estado durable y reintentar misma clave |
| Espera previa a Benefits agota Gateway | 504 TIMEOUT | Misma clave; en este caso no hubo movimiento antes del reintento |
| Respuesta retenida tras commit | 504 TIMEOUT | El movimiento ya existe; misma clave devuelve 201, replay y el cuerpo original exacto |

OpenAPI sigue en 1.1.1 y ADR-008 v3: no se cambian códigos, endpoints ni versión del contrato. Las protecciones funcionales estaban implementadas en etapas 3 y 4; etapa 6 agrega comprobación reproducible y evidencia.

## Medición aprobada y ejercicio

D-56 aprueba hasta 500 ms adicionales solo al medir de punta a punta. El código mantiene 4 s y 5 s; el margen admite transporte y planificación local y no es un SLA ni un valor del teórico. No se lo usa para ampliar el contexto de ejecución.

El ejercicio usa el stack local, fixtures sintéticos y un proxy temporal escrito con la biblioteca estándar de Go. El proxy bloquea antes de Benefits o consume una respuesta 201 completa y la retiene hasta que Gateway cancela. Su override de Compose se usa únicamente para pruebas, no agrega endpoints a la aplicación y se retira al finalizar. Los logs del inyector conservan estado, trace_id y hash del cuerpo; nunca el cuerpo o las credenciales. Reutilizar Go permite generar la falla exacta sin incorporar un producto externo.

El script restaura Benefits, PostgreSQL y la ruta directa en finally, y exporta evidencia solo después de verificar la restauración. Las trazas y logs se preservan antes de recrear el Gateway. El ejercicio necesita un entorno local sin otro uso concurrente; pausar la instancia PostgreSQL afecta temporalmente también a los otros servicios SQL del gimnasio.

## Fundamento y límites

Teórico: presupuestos, cancelación e idempotencia, pp. 101–105 y 113–116; transacciones locales, pp. 27–28; correlación/trazabilidad, pp. 7–8. Los 4 s/5 s y el margen de 500 ms son decisiones propias aprobadas; el PDF no prescribe esos valores ni las herramientas de observabilidad.

La evidencia corresponde a un ejercicio local: no cubre todas las particiones de red, fallas del host, saturación prolongada ni respuesta de un proveedor externo. No se añaden alertas activas o SLO validados. Un 503/504 en general no prueba ausencia de commit. En cada caso se consulta el estado durable antes de reintentar.

## Verificación

Tests: Gateway no reintenta un POST ante 503; cancelación previa al commit revierte movimiento, saldo y resultado con PostgreSQL real; el inyector retiene una sola respuesta. Ejercicio: 502, 503 y 504 antes/después de commit, comparación del hash del cuerpo original y del replay y recuentos de base. Evidencia y comandos en [ETAPA_6_RESILIENCIA.md](../entrega-2/ETAPA_6_RESILIENCIA.md) y [POSTMORTEM.md](../POSTMORTEM.md).
