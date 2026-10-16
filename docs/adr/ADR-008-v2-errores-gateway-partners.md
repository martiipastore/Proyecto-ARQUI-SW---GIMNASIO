# ADR-008 v2 — Incluir las fallas del gateway en el contrato de partners

| Campo | Valor |
|---|---|
| **Estado** | Aceptado; contrato actualizado, implementación del gateway pendiente de etapa 3 |
| **Fecha** | 2026-10-16 |
| **Decisión del TP** | D8 |
| **Reemplaza** | [ADR-008](ADR-008-contrato-propio.md) |
| **Autores** | Equipo del TPI |
| **Relacionado con** | RN-26, RN-27; ADR-005 v2; ADR-010; docs/contracts/README.md |

## Contexto

El contrato 1.0.0 documentaba errores de Benefits pero no los HTTP 502/504 que el gateway puede generar antes de obtener respuesta. El gateway actual usa el catálogo interno y correlation_id, mientras que partners espera códigos en inglés y correlationId. El equipo aprobó alinear el contrato el 16/10/2026.

## Alternativas consideradas

| Alternativa | Ventaja | Desventaja |
|---|---|---|
| Dejar los errores de borde fuera del contrato | Sin cambios de versión | El consumidor no conoce todas las fallas posibles |
| Convertir todas las fallas a 503 | Un único error temporal | Pierde la distinción entre conexión fallida y espera agotada acordada en D5 |
| Documentar 502/504 con formato de partners | Mantiene D5 y un formato consistente | Exige adaptar el gateway y tests contractuales |

## Decisión

Mantener todas las decisiones de ADR-008 salvo la ampliación del catálogo: publicar OpenAPI **1.1.0**, conservando rutas /v1, con 502 UNAVAILABLE cuando el gateway no alcanza Benefits y 504 TIMEOUT cuando agota la espera. Ambos usan application/problem+json, correlationId y X-Correlation-Id; 502 incluye Retry-After. El consumidor puede reintentar con la misma Idempotency-Key y backoff. Un timeout no prueba que la operación haya sido revertida.

Es un cambio menor según la política publicada: se añaden respuestas y un código de error sin modificar solicitudes ni respuestas de éxito. Los consumidores deben manejar códigos desconocidos por status. No se cambia retención, aislamiento, autenticación ni límites de puntos.

El límite inicial de la operación de Benefits es 4 s dentro de los 5 s del gateway, según la aprobación registrada en ADR-010. Es presupuesto operativo, no un cambio en el timeout público de 5 s.

Los errores que nacen en el gateway no se guardan como si Benefits hubiera recibido la operación. Si existe resultado durable en Benefits, la misma clave lo recupera. La implementación debe distinguir el formato público de partners del formato interno sin incorporar reglas de negocio al gateway.

## Consecuencias

- Positiva: el consumidor puede manejar las fallas del flujo completo con un formato documentado.
- Negativa: el proxy actual todavía debe traducir los errores de borde al catálogo y formato públicos.
- Riesgo: un consumidor que trate el enum como cerrado puede rechazar TIMEOUT; la política contractual exige tolerar nuevos códigos.

## Cómo se verificará

Spectral sobre OpenAPI 1.1.0 y ejemplos 502/504. En etapa 3, tests a través del gateway con Benefits inaccesible y lento: status, código, formato camelCase, correlación y Retry-After donde aplica. En etapa 6, reintento tras resultado indeterminado sin duplicar movimientos. No se afirma que estas pruebas de comportamiento ya estén implementadas.
