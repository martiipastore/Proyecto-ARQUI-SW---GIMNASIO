# ADR-008 v4 — Alcance operativo de la capacidad compartida

| Campo | Valor |
|---|---|
| Estado | Propuesto; cambio preparado para revisión del equipo y despliegue pendiente |
| Fecha | 2026-11-08 |
| Decisión del TP | D8 — Contrato propio |
| Relacionado con | ADR-008 v3, ADR-018, SPEC Club, OpenAPI v2 |

## Contexto

El contrato 1.4.0 contenía rutas de débitos, catálogo y canjes que nunca tuvieron
implementación real. El equipo decidió no realizar esas tres operaciones. Mantenerlas
en el contrato operativo permitiría que otro grupo desarrollara contra el mock y
fallara al llamar al proveedor real. La v1 queda preservada como antecedente; no
se borra la historia de las decisiones previas ni sus evidencias.

## Decisión propuesta

La capacidad ofrecida comprende cinco operaciones: registrar o vincular a una
persona, consultar el recibo de ese registro, acreditar puntos y consultar saldo
y movimientos. Se publica un contrato OpenAPI 2.0.0 con ruta pública
`/partner-api/v2`, guía y mock acordes. El gateway y Benefits aceptan esa ruta;
`/partner-api/v1` sigue disponible localmente para compatibilidad, pero su
contrato histórico incluía operaciones sin implementar y no se ofrece a nuevos
consumidores.

Una integración nueva envía `eventId` en cada crédito y `Idempotency-Key` en cada
escritura. La cuota inicial se documenta como 120 solicitudes por minuto por
integración, configurable en PostgreSQL. Registro e identidad requieren habilitar
sus flags y configurar claves y correo antes de publicar el entorno.

## Consecuencias y verificación

El otro grupo solo recibirá el contrato v2 y ejemplos de las operaciones reales.
El despliegue HTTPS, la provisión de credenciales, el correo productivo y las
pruebas desde otra red siguen pendientes y no se acreditan por este ADR.

El cambio se verifica con Spectral, tests de rutas v2 en gateway y Benefits,
pruebas funcionales existentes del ledger/registro y, antes de entregar la URL,
un recorrido completo contra el servicio real desde una red externa.

El 08/11/2026 pasó el [E2E local de v2](../evidence/club/cb04-v2-e2e.json):
14 escenarios con Gateway, Benefits, Members, PostgreSQL 16 y Mailpit reales.
No se probó un despliegue público ni correo productivo.
