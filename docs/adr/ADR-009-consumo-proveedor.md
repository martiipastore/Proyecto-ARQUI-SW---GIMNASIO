# ADR-009 — Adaptar la capacidad del proveedor desde el servicio consumidor

| Campo | Valor |
|---|---|
| **Estado** | Propuesto; pendiente de proveedor y aprobación del equipo |
| **Fecha** | 2026-10-16 |
| **Decisión del TP** | D9 |
| **Autores** | Equipo del TPI |
| **Relacionado con** | SPEC §1.5; ADR-002 v2; ADR-005 v2; ADR-010 |

## Contexto

El enunciado §4 exige consumir una capacidad de otro grupo con consecuencias de negocio. SPEC §1.5 deja pendientes proveedor, capacidad y flujo. Este documento identifica lo que falta; no afirma que exista integración ni selecciona un proveedor.

El 16/10/2026 el equipo confirmó que todavía no recibió la asignación y que la recibirá próximamente. D9 permanece como propuesta inicial; la etapa 5 depende de esa asignación y del contrato publicado por el proveedor.

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
| Grupo, contacto y capacidad asignada | Asignación aún no recibida, confirmado por el equipo el 16/10/2026 |
| Contrato, versión, URL y entorno | Pendiente |
| Servicio consumidor, HU/RN y efecto observable | Pendiente; requiere aprobación del alcance |
| Autenticación y gestión de secretos | Pendiente del contrato |
| Timeout, reintentos y presupuesto total | Pendiente del flujo; no copiar valores sin justificar |
| Cuotas, límites y costos | Pendiente de información verificable del proveedor |
| Traducción de errores y cambios incompatibles | Pendiente del contrato |
| Falla cerrada o degradación visible | Pendiente del impacto de negocio |

## Consecuencias

- Positiva: el dominio no depende del formato ajeno; se puede probar el adaptador.
- Negativa: el cierre de D9 depende de información externa y de una definición funcional.
- Riesgo: un mock inventado daría falsa evidencia. Mitigación: no implementarlo antes de recibir el contrato.

## Cómo se verificará

En la etapa 5: contrato recibido, test del adaptador y de compatibilidad, respuesta normal, error y caída del proveedor con comportamiento visible. La defensa debe mostrar qué decisión del negocio utiliza la capacidad. Hasta entonces D9 sigue propuesta y su implementación pendiente.
