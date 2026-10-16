# ADR-014 — Mantener la búsqueda de clases en OpenSearch mediante eventos versionados

| Campo | Valor |
|---|---|
| **Estado** | Aceptado como diseño inicial el 16/10/2026; implementación y mediciones pendientes |
| **Fecha** | 2026-10-16 |
| **Decisión del TP** | D6 |
| **Autores** | Equipo del TPI |
| **Relacionado con** | SPEC HU-13, HU-14, RN-14; ADR-002 v2; ADR-003; ADR-005 v2; ADR-007 |

## Contexto

El enunciado identifica D6 como búsqueda. ADR-006 documenta seguridad y tiene una etiqueta D6 histórica incorrecta; se conserva su decisión de seguridad y se registra aquí la decisión de búsqueda con número de archivo independiente del identificador del TP.

El diseño ya elige OpenSearch como modelo de lectura de Clase y PostgreSQL como fuente de cupo y reservas. Un listado puede estar atrasado; reservar debe verificar las reglas con datos transaccionales.

## Alternativas consideradas

| Alternativa | Ventaja | Desventaja |
|---|---|---|
| Consultar solo PostgreSQL | Menos componentes | No cumple el motor de búsqueda requerido |
| Escribir PostgreSQL y OpenSearch directamente en la petición | Visibilidad rápida si ambos responden | Falla parcial puede dejar estados divergentes sin recuperación durable |
| Outbox, evento versionado e indexer | Reintento durable e índice reconstruible | Consistencia eventual y operación adicional |

## Decisión

Formalizar OpenSearch 2, índice clases y alias clases-read, un documento por clase con actividad, fecha, inicio/fin, franja, profesor, capacidad, ocupación, estado y versión. booking-indexer consume clase.actualizada publicado mediante outbox en la misma transacción del cambio de Booking. Solo aplica versiones más nuevas y deduplica reentregas.

Búsqueda con filtros por actividad, fecha/rango, franja y disponibilidad; paginación y orden por inicio ascendente con identificador como desempate estable. Página de 20 elementos por defecto, máximo 100, aprobado por el equipo el 16/10/2026; el contrato de búsqueda definirá su límite de profundidad antes de implementar. No se cambia el contrato de partners.

Retraso objetivo < 2 s desde commit en PostgreSQL hasta visibilidad en búsqueda durante operación normal. Una caída puede excederlo: debe reportarse, reintentarse y reconciliarse; no se promete ese límite durante indisponibilidad. Cupo, permisos y reserva se deciden siempre contra la fuente vigente.

Reindexado desde booking_db por su propio indexer hacia un índice nuevo y cambio de alias al completar y verificar convergencia con eventos concurrentes. La estrategia detallada de captura de cambios durante reindexado se resolverá antes de implementarlo. OpenSearch caído produce degradación parcial de Booking; no se inventa una búsqueda alternativa silenciosa.

## Consecuencias

- Positiva: búsquedas no compiten directamente con escrituras y admiten reconstrucción.
- Negativa: componentes adicionales y vista temporalmente atrasada.
- Riesgo: reentregas fuera de orden o eventos fallidos. Mitigación: versiones, reintentos, DLQ y medición del retraso.

## Cómo se verificará

En la etapa de búsqueda: tests de filtros, paginación y desempate; evento viejo no pisa uno nuevo; edición visible en < 2 s en el escenario controlado; caída, recuperación y reindexado sin perder cambios concurrentes. Esta etapa solo entrega el diseño inicial, no un motor ya desplegado.
