# Revisión inicial de decisiones — Entrega 2

Fecha: 16/10/2026. Alcance: etapa 1 del plan. Revisión documental y del esqueleto; no acredita implementación del dominio ni pruebas nuevas de resiliencia, carga o trazas.

Criterio académico aclarado por el equipo: conceptos basados en clase y herramientas adicionales permitidas si son necesarias, útiles y justificadas en decisiones/etapas. Ver [BASE_TEORICA.md](BASE_TEORICA.md). Se conservan las aprobaciones de D11 y D12 con esa justificación. D13 tiene plan inicial aprobado con k6 como herramienta adicional justificada; verificaciones consecutivas aprobadas según p. 87: ADR-012 v2.

## Decisiones existentes

| Decisión | Resultado de la revisión | Evidencia y trabajo posterior |
|---|---|---|
| D1 | Se mantiene Benefits como dueño de cuentas, saldo, movimientos, beneficios, canjes y partners | ADR-001 y SPEC RN-22/26. La identidad interna de cuenta no se confunde con la vinculación partner/externalUserId |
| D2 | Se mantiene Hexagonal | ADR-002 v2. AcreditarPuntos, ConsultarSaldo y ListarMovimientos en application; reglas y errores en domain; repositorios/reloj/idempotencia en ports; HTTP y PostgreSQL en adapters. cmd realiza cableado. Tests de arquitectura ya pasan; pruebas de negocio en etapa 2 |
| D3 | Se mantiene PostgreSQL en benefits_db | ADR-003/004. Vinculación por (partner, externalUserId); cuenta por ID interno; lectura de saldo total; movimientos por cuenta y partner, fecha descendente y paginación; unicidad de (partner, key), hash y respuesta durable ≥ 30 días. Bloqueo de cuenta y transacción única para saldo, movimiento e idempotencia. Índices y migraciones en etapa 2 |
| D4 | Se conserva la serialización por cuenta | ADR-004. El orden de aplicación es account_sequence; la fecha del hecho no recalcula el ledger. El estado REVERTIDO es la transición explícita permitida por RN-22; no habilita editar importe ni borrar historia |
| D5 | Se mantiene Partner → Gateway → Benefits, 5 s, sin reintentos de aplicación en gateway | ADR-005 v2 y configuración/proxy actuales. El partner recupera con la misma clave; falta alinear errores de borde con D8 y adaptar el prefijo público a /v1 en etapa 3 |
| D7 | Saldo e idempotencia no dependen de Redis | ADR-007 solo permite caché de catálogos y contadores. API key, partner y vinculación se validan con fuente vigente. La medición de caché corresponde al flujo de catálogos futuro |
| D8 | Operaciones mínimas y payloads ya están definidos | OpenAPI 1.1.0 cubre créditos, saldo y movimientos, replay, aislamiento y errores. Límite técnico 1–1.000.000 según D-26; no se redefine como límite de negocio. Errores 502/504 documentados; traducción del gateway pendiente de etapa 3 |

## Compatibilidad de errores de partners aprobada el 16/10/2026

El equipo aprobó conservar los HTTP 502/504 previstos en D5 y el plan, agregar esas respuestas al OpenAPI y usar el formato de partners también cuando el error nace en el gateway:

| Situación | HTTP | code aprobado | Formato |
|---|---|---|---|
| No se alcanza Benefits | 502 | UNAVAILABLE (ya existe) | Problem de partners con correlationId |
| Timeout del gateway | 504 | TIMEOUT (nuevo en catálogo de partners) | Problem de partners con correlationId |

Ambos permiten reintento con la misma Idempotency-Key; 504 no implica rollback. Para 502 se incluye Retry-After conforme a la política de indisponibilidad; no se inventa una respuesta persistida cuando el request no llegó a Benefits.

Se registró ADR-008 v2 (aceptado), se marcó el original como reemplazado y se actualizó OpenAPI a 1.1.0 según la política de versionado, junto con el CHANGELOG. La implementación y prueba del proxy quedan pendientes de etapa 3. El catálogo interno conserva sus nombres en español.

## Estado de los ADR revisados por el equipo

| Archivo | Decisión del TP | Propuesta y evidencia pendiente |
|---|---|---|
| [ADR-014](adr/ADR-014-busqueda-clases.md) | D6 | Diseño inicial aprobado el 16/10/2026: OpenSearch/Clase, filtros, orden por inicio, páginas de 20 (máximo 100) y objetivo de lag < 2 s; implementación y medición pendientes |
| [ADR-009](adr/ADR-009-consumo-proveedor.md) | D9 | Puerto y adaptador desde el servicio responsable; el equipo confirmó el 16/10/2026 que aún no recibió la asignación y la recibirá próximamente. Faltan proveedor, contrato y flujo |
| [ADR-010](adr/ADR-010-resiliencia-benefits.md) | D10 | Diseño inicial aprobado el 16/10/2026: gateway 5 s, Benefits 4 s, cero retry de escritura en gateway, recuperación con la misma clave, fallo cerrado y sin circuit breaker en este flujo; implementación y caída pendientes |
| [ADR-011](adr/ADR-011-observabilidad-v1.md) | D11 | Diseño inicial aprobado el 16/10/2026: logs JSON, OTel → Collector → Jaeger, muestreo local 100 %, métricas y umbrales iniciales; instrumentación, traza y validación pendientes |
| [ADR-012 v2](adr/ADR-012-v2-estabilidad-health-checks.md) | D12 | Diseño inicial aprobado el 16/10/2026: dos Booking, Traefik con pesos iguales, sin afinidad y checks cada 5 s/timeout 2 s, retiro tras 3 fallos y regreso tras 2 éxitos; soporte de herramienta, implementación y distribución pendientes |
| [ADR-013](adr/ADR-013-capacidad-costos.md) | D13 | Plan inicial aprobado el 16/10/2026: k6 como herramienta adicional justificada, escenarios y objetivos p95 iniciales; mediciones y cotización pendientes |

## Alineación documental

- D6 del TP es búsqueda, no seguridad. ADR-006 conserva su número, texto e historia; el índice aclara que es una decisión adicional y que su etiqueta D6 histórica es incorrecta. ADR-014 cubre búsqueda.
- D-41 ya fija Oracle Cloud Free Tier/VM ARM; se corrige ARCHITECTURE §14.2 para reflejar la decisión confirmada, sin afirmar recursos gratuitos disponibles ni despliegue existente.
- D-42 confirma que Benefits alcanza como servicio operativo y capacidad compartida para esta entrega.
- ADR aceptados no se reescriben para incorporar decisiones nuevas: se usarán reemplazos cuando cambie su decisión. Las propuestas nuevas no equivalen a aprobación ni implementación.

## Cierre documental de la etapa 1

### Estabilidad del balanceo aprobada

El equipo aprobó 3 fallos consecutivos para retirar y 2 éxitos consecutivos para reincorporar, manteniendo intervalo de 5 s y timeout de 2 s. Fundamento: p. 87 del teórico, evitar flapping por resultados aislados. Los conteos son elecciones del proyecto y su costo es mayor demora de detección/recuperación. ADR-012 v2 reemplaza al original; D-51 conserva la decisión y su justificación en el registro. Soporte de la herramienta e implementación quedan para la etapa de balanceo. No se añadirá otro componente silenciosamente si la herramienta no lo soporta.

- D6, D10, D11, D12 y D13 tienen diseño o plan inicial aprobado; D9 espera la asignación del proveedor.
- Errores de partners aprobados y documentados en ADR-008 v2/OpenAPI 1.1.0; Spectral sin advertencias. Implementación del proxy pendiente de etapa 3.
- D9 seguirá explícitamente inicial mientras falten la asignación y el contrato. La etapa 5 continuará bloqueada; no se presentará como integración implementada.
- Revisión final de enlaces e identificadores ejecutada; se conserva pendiente la revisión por otro integrante del cambio del SPEC antes de integrar mediante PR. No se realizaron commits ni publicación.

### Hallazgo de la revisión final: definición del saldo

El glosario de SPEC define saldo como suma de movimientos confirmados, y la última verificación de ADR-004 usa esa misma expresión. Sin embargo, RN-22 y CA-19.2 indican sumar todos los movimientos, incluidos los compensatorios. CA-11.1 exige que el original pase a REVERTIDO al insertar un compensatorio de signo contrario.

Ejemplo: crédito +100, reversión −100, saldo esperado 0. Si la consulta excluye el crédito original por su estado REVERTIDO, obtiene −100 y contradice RN-22, CA-11.1 y la regla de saldo no negativo.

Aclaración aprobada por el usuario el 16/10/2026: aclarar el glosario y el criterio de verificación para sumar todos los importes registrados, incluido el original REVERTIDO y su compensatorio. REVERTIDO impide una segunda reversión; no elimina el aporte histórico del original. Conservar la historia del ADR aceptado mediante una revisión. No cambiar fórmulas ni documentos aceptados silenciosamente. Se preparó SPEC v0.10 y ADR-004 v2; el original conserva su historia como reemplazado. El cambio del SPEC queda pendiente de revisión por otro integrante según CONTRIBUTING; no se publicó ni integró.

El hallazgo quedó resuelto en los documentos locales con D-52 y ADR-004 v2. No cambia la asignación pendiente del proveedor ni habilita implementar reversiones fuera del flujo mínimo de la etapa 2.

## Resultado final

Etapa 1 completada en el workspace el 16/10/2026: decisiones obligatorias identificadas, aprobaciones iniciales documentadas, D9 propuesta explícita pendiente del proveedor, contrato 1.1.0 validado con Spectral y aclaración del saldo preparada. Implementación, mediciones, soporte de health checks y aprobación de PR del SPEC siguen pendientes en sus instancias correspondientes. Este cierre no acredita aprobación de otro integrante, merge ni entrega publicada.
