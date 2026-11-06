# Etapa 8 — Revisión de la Entrega 2

Fecha: 16/10/2026. Estado: **en curso**. Base: `origin/main` con PR #5 integrado (`fb2a17c`), que incluye los commits de etapa 7 `aabe4e0` y `05f7c0e`. Esta revisión no declara finalizado el proyecto ni sustituye la asignación del proveedor.

## Contraste con el enunciado

Fuente: `Enunciado TP Final.pdf`, sección 6.3, página 8. La Entrega 2 pide dominio propio de punta a punta, persistencia, logs correlacionados, primera traza y decisiones actualizadas. Frontend completo, capacidad pública en nube y proveedor incorporado al flujo figuran en la presentación grupal posterior. El plan del equipo incluye la integración externa en etapa 5, que continúa pendiente.

| Requisito de Entrega 2 | Evidencia y estado |
|---|---|
| Servicio operativo y funcionalidad compartida funcionando | Benefits acredita puntos, consulta saldo y lista movimientos por la API pública. D-42 registra la aclaración de cátedra comunicada por el equipo: Benefits alcanza para ambos roles. La aclaración no se presenta como texto literal del PDF |
| Al menos un almacenamiento en el flujo | PostgreSQL, migraciones versionadas, ledger e idempotencia durable. Etapas 2/3 |
| Logs estructurados y correlacionados; primera traza distribuida | Gateway → Benefits → PostgreSQL, logs JSON, Collector y Jaeger. Etapa 4 y repetición de demostración en esta revisión pendiente |
| D2, D6, D7, D9, D10, D12 y D13 | ADR presentes y enlazados abajo. D9 sigue incompleta por proveedor/contrato sin asignar; no se equipara una propuesta genérica con una decisión informada |
| Validación de D1, D3 y D5 | Revisión del código, pruebas y límites de implementación, tabla siguiente |
| Primera versión D11 | ADR-011 v2 y primera traza; métricas continuas, tablero y alerta operativa siguen pendientes |
| D8 actualizada si cambia contrato | OpenAPI 1.1.1 y ADR-008 v3. Esta etapa no redefine APIs ni cambia su versión |
| Bonus anunciados | D-40: Playwright para flujos del SPEC y ocupación en tiempo real con SSE. Anunciados, no implementados |

## Ampliación posterior del Club — 06/11/2026

La evidencia de esta etapa pertenece al flujo previo y no valida el código ampliado
de registro, activación, sesiones, correo o cuotas. El contrato local ahora es
1.4.0; la referencia 1.1.1 en la revisión histórica siguiente conserva su contexto.
El nuevo trabajo se sigue en [PLAN_CLUB_BENEFICIOS.md](PLAN_CLUB_BENEFICIOS.md),
D-63 a D-70 y ADR-016/017/018. Debe repetirse la validación ampliada con base/correo
reales y Spectral antes de declarar cerrado ese alcance. No se cambia el estado
histórico de los experimentos ni se da D9 por resuelta.

## Revisión de decisiones y alcance real

Los ADR aceptados se conservan como historia; esta tabla verifica su aplicación actual sin reescribir el diseño objetivo como si estuviera implementado.

| Decisión | Revisión | Evidencia y límite |
|---|---|---|
| D1 — [límites](adr/ADR-001-limites-de-servicios.md) | Se mantienen responsabilidades y propiedad de datos | Benefits accede solo a `benefits_db`; no se fusionó con Booking. Los demás flujos de negocio siguen pendientes |
| D2 — [patrones](adr/ADR-002-v2-patrones-internos.md) | Hexagonal reconocible en Benefits | Dominio, aplicación, puertos, HTTP/PostgreSQL y cableado separados; tests de arquitectura. No se afirma que todos los casos de todos los servicios estén completos |
| D3 — [persistencia](adr/ADR-003-persistencia.md) | PostgreSQL participa en el flujo real | Restricciones, transacción de saldo/movimiento/operación, índices y retención. Roles por base definidos en init; no se atribuye implementación funcional a Redis/OpenSearch por tenerlos en el diseño |
| D5 — [comunicación](adr/ADR-005-v2-comunicacion.md) | Se verifica el recorrido síncrono de partners | Gateway 5 s, Benefits 4 s, sin reintentos automáticos de escritura; replay conserva respuesta confirmada. Outbox, eventos y consumidores son diseño futuro |
| D6 — [búsqueda](adr/ADR-014-busqueda-clases.md) | Diseño documentado | OpenSearch, filtros, orden, paginación y lag objetivo; implementación y medición pendientes |
| D7 — [caché y disponibilidad](adr/ADR-007-cache-autorizacion-disponibilidad.md) | Flujo mínimo no cachea autorización ni saldo | Credencial, partner y vinculación consultan PostgreSQL; indisponibilidad falla cerrada. Catálogos cacheados y comparación antes/después pendientes |
| D8 — [contrato](adr/ADR-008-v3-fecha-cuenta-sin-movimientos.md) | Contrato publicado 1.1.1, sin cambio en esta etapa | Tres endpoints reales; resto disponible como contrato/mock, no como negocio implementado. Contrato y guía mantienen CHANGELOG existente |
| D9 — [proveedor](adr/ADR-009-consumo-proveedor.md) | Propuesta incompleta, pendiente de asignación | Faltan grupo, capacidad, contrato, límites y comportamiento específico. No se inventa un proveedor ni un contrato |
| D10 — [resiliencia](adr/ADR-010-v2-resiliencia-validada.md) | Presupuestos y recuperación demostrados en etapa 6 | 502/503/504, respuesta perdida tras commit y recuperación sin duplicación; repetición final pendiente |
| D11 — [observabilidad](adr/ADR-011-v2-observabilidad-local.md) | Primera versión operativa | Logs y trazas exportadas; Jaeger volátil. Umbrales propuestos no son alertas desplegadas |
| D12 — [balanceo](adr/ADR-012-v2-estabilidad-health-checks.md) | Diseño aprobado | Dos Booking y checks 3/2 definidos; hoy una instancia. Soporte, distribución y pruebas reales pendientes de la etapa correspondiente |
| D13 — [capacidad/costos](adr/ADR-013-v2-capacidad-costos-medidos.md) | Evidencia local revisada | 36 fases, invariantes y contención observada; costos fechados São Paulo USD/ARS. No equivale a capacidad ARM ni disponibilidad gratuita garantizada |

## Verificación final

Tests Go de siete módulos secuenciales sin caché (`-count=1`), `go vet`, tests del inyector, diagnóstico de pool, TypeScript, build frontend y Spectral: correctos. Golangci-lint v2.1.6, misma versión de CI: en curso. Evidencia: [validation.json](evidence/etapa8/validation.json). Una etapa no se marca completa únicamente por disponer de un checklist.

Para la prueba desde cero se propuso usar una copia aislada con volúmenes nuevos, preservando el gimnasio actual. Aprobado como D-61: se implementó `tests/e2e/review-delivery.ps1` y se está ejecutando. No se borran volúmenes originales ni de prueba; se comprueban tablas nuevas vacías y se exporta evidencia antes de restaurar.

La instalación del frontend detectó avisos de seguridad de Vite/esbuild. Actualizar Vite 5.4.11 a una versión corregida implica una decisión de versión mayor, consultada al equipo; aprobada como D-62. Vite 6.4.3 ya pasó TypeScript y build, con cero vulnerabilidades reportadas por npm; la comprobación Docker está en curso. No se usa `npm audit fix --force` automáticamente. La compatibilidad de Node 20 y plugin React se comprobó con metadata del registro npm; [resumen](evidence/etapa8/frontend-audit.json). La [advertencia oficial de Vite](https://github.com/vitejs/vite/security/advisories/GHSA-fx2h-pf6j-xcff) incluye versiones hasta 6.4.2 y señala 6.4.3 corregida.

## Fundamento y pendientes de cierre

El fundamento académico de los mecanismos se mantiene en [BASE_TEORICA.md](BASE_TEORICA.md) y en cada ADR. Esta etapa revisa trazabilidad entre requisito, decisión y evidencia; no incorpora un concepto nuevo de arquitectura por iniciativa del asistente. Herramientas adicionales previamente autorizadas siguen identificadas como tales.

Pendientes: validación limpia y repetición del flujo/caída, resultados finales, guion de demostración, sincronización de registro/plan y commit. D9/etapa 5 debe resolverse cuando se reciba la asignación; se mantiene visible como impedimento para cerrar todo el plan de entrega.
