# ADR-010 — Limitar esperas y recuperar escrituras mediante idempotencia

| Campo | Valor |
|---|---|
| **Estado** | Reemplazado por [ADR-010 v2](ADR-010-v2-resiliencia-validada.md) el 16/10/2026; se conserva el diseño inicial |
| **Fecha** | 2026-10-16 |
| **Decisión del TP** | D10 |
| **Autores** | Equipo del TPI |
| **Relacionado con** | RN-22, RN-23, RN-27; CL-08 y CL-08b; ADR-004; ADR-005 v2; ADR-007; ADR-008 |

## Contexto

El flujo de la entrega es Partner → Gateway → Benefits → PostgreSQL. Una respuesta perdida no indica si la transacción se confirmó. Repetir automáticamente la escritura con otra clave puede duplicar puntos; esperar indefinidamente retiene recursos.

## Alternativas consideradas

| Alternativa | Ventaja | Desventaja |
|---|---|---|
| Reintentar escrituras en gateway | Oculta algunas fallas | Amplía carga y ambigüedad ante commit sin respuesta |
| Timeout y reintento explícito con la misma clave | Presupuesto acotado y recuperación durable | El consumidor debe manejar el resultado indeterminado |
| Añadir circuit breaker a este flujo desde el inicio | Reduce llamadas durante fallas sostenidas | Añade estados y umbrales sin medición inicial |

## Decisión

Conservar el timeout total de 5 s del gateway para partners y cero reintentos de aplicación en el gateway (ADR-005 v2). Benefits propagará cancelación a PostgreSQL y acotará la operación de aplicación/base a 4 s, o al tiempo restante del contexto si es menor. El equipo aprobó ese límite inicial el 16/10/2026; falta implementarlo y comprobar su suficiencia.

PostgreSQL es crítica: si no puede confirmar la operación, Benefits falla cerrado. No se acredita en memoria ni se usa Redis como respaldo. Movimiento, saldo y resultado idempotente se confirman en una transacción. Un commit cuyo resultado se desconoce se resuelve consultando/reintentando con la misma clave, nunca suponiendo rollback.

| Falla | Respuesta prevista | Recuperación |
|---|---|---|
| Gateway no conecta con Benefits | 502 | Misma clave tras restaurar servicio |
| Se agota el presupuesto del gateway | 504 | Resultado indeterminado; misma clave |
| PostgreSQL no disponible en Benefits | 503 UNAVAILABLE con Retry-After | Misma clave; aplicar el contrato de replay si ya hay resultado durable |
| Operación de negocio rechazada y registrada | Rechazo original | Misma clave reproduce rechazo; nuevo intento de negocio requiere otra |

ADR-008 v2 y OpenAPI 1.1.0 documentan 502 UNAVAILABLE y 504 TIMEOUT con formato de partners, aprobados el 16/10/2026. El gateway todavía usa códigos técnicos compartidos: su traducción se implementará y probará en etapa 3.

El equipo aprobó no incorporar circuit breaker para Gateway → Benefits en este flujo inicial. Las protecciones previstas para llamadas internas a members conservan su diseño existente y quedan fuera de esta implementación. D9 tendrá su propia política cuando se conozca el proveedor.

## Consecuencias

- Positiva: la falla corta dentro del presupuesto y permite recuperar un resultado sin duplicar movimientos.
- Negativa: un timeout puede dejar al consumidor sin certeza temporal sobre el resultado.
- Riesgo: contención de cuenta o pool puede consumir el presupuesto. Mitigación: transacciones cortas, cancelación y pruebas de carga.

## Cómo se verificará

Etapa 6: detener Benefits, hacer lenta/no disponible PostgreSQL y provocar pérdida de respuesta tras commit. Medir tiempo, verificar 502/504/503 y comparar movimientos antes y después del reintento con la misma clave. Restaurar dependencias y comprobar recuperación. Documentar evidencia y línea temporal en POSTMORTEM; no existe todavía evidencia de este ejercicio.
