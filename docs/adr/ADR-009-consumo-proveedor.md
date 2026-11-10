# ADR-009 — Adaptar la capacidad del proveedor desde el servicio consumidor

| Campo | Valor |
|---|---|
| **Estado** | Propuesto; manual de MELT recibido el 10/11/2026; flujo, servicio consumidor y aprobación pendientes |
| **Fecha** | 2026-10-16 |
| **Decisión del TP** | D9 |
| **Autores** | Equipo del TPI |
| **Relacionado con** | SPEC §1.5; ADR-002 v2; ADR-005 v2; ADR-010 |

## Contexto

El enunciado §4 exige consumir una capacidad de otro grupo con consecuencias de negocio. SPEC §1.5 requiere concretar el flujo. Este documento identifica lo que falta; no afirma que exista integración ni aprueba un servicio consumidor.

El 16/10/2026 todavía no se había recibido la asignación. El 10/11/2026 el equipo entregó el manual de **MELT** y confirmó que es toda la información recibida. Describe catálogo público, registro referido, login y puntos MELT. El documento, su procedencia y sus pendientes están en [ETAPA_13_MELT.md](../entrega-final/ETAPA_13_MELT.md). D9 permanece propuesto: la implementación corresponde a la etapa 13 del plan final (la referencia histórica a etapa 5 era de la planificación anterior).

## Alternativas consideradas

| Alternativa | Ventaja | Desventaja |
|---|---|---|
| Llamar desde frontend o gateway | Entrada directa | Contradice el enunciado y desplaza la responsabilidad de negocio |
| Usar el contrato externo en el dominio | Menos mapeos iniciales | Acopla reglas internas a cambios ajenos |
| Puerto propio y adaptador en el servicio consumidor | Aísla contrato y permite pruebas | Requiere traducción y mantenimiento |

## Decisión propuesta

El servicio responsable del caso de uso consumirá la capacidad mediante un puerto con vocabulario propio y un adaptador de salida. El contrato versionado recibido se conservará con su procedencia; un test contractual deberá detectar cambios incompatibles. Un fake basado en ese contrato permitirá pruebas sin el proveedor, sin presentarlo como integración real.

Antes de aceptar esta decisión se completará:

| Dato | Estado |
|---|---|
| Grupo, contacto y capacidad asignada | MELT; catálogo de tratamientos y registro referido con beneficio de 100 puntos en MELT. Contacto nominal no informado. Flujo propio por definir. |
| Contrato, versión, URL y entorno | Manual PDF de 5 páginas recibido el 10/11, sin versión explícita; base declarada https://melt-gateway.onrender.com, prefijo /api/v1. Sin verificación en vivo. |
| Servicio consumidor, HU/RN y efecto observable | Pendiente; requiere aprobación del alcance |
| Autenticación y gestión de secretos | Catálogo y registro públicos; login con credenciales personales MELT y consulta de puntos con Bearer de cliente. No se entregan secretos reales. Código de referido definitivo pendiente; no confundirlo con una credencial. |
| Timeout, reintentos y presupuesto total | Pendiente del flujo; no copiar valores sin justificar |
| Cuotas, límites y costos | Pendiente de información verificable del proveedor |
| Traducción de errores y cambios incompatibles | El manual documenta HTTP status y campo externo error variable; esquemas completos y traducción al contrato propio pendientes. |
| Falla cerrada o degradación visible | Pendiente del impacto de negocio |

## Consecuencias

- Positiva: el dominio no depende del formato ajeno; se puede probar el adaptador.
- Negativa: el cierre de D9 depende de información externa y de una definición funcional.
- Riesgo: un mock inventado daría falsa evidencia. Mitigación: no implementarlo antes de recibir el contrato.

## Cómo se verificará

En la etapa 13 del plan final: completar el contrato recibido, test del adaptador y de compatibilidad, respuesta normal, error y caída del proveedor con comportamiento visible. La defensa debe mostrar qué decisión del negocio utiliza la capacidad. Hasta entonces D9 sigue propuesta y su implementación pendiente. El manual contempla llamadas desde navegador y una eventual llamada inversa, pero no autoriza cambiar la arquitectura propia ni inventa un callback MELT → gimnasio.
