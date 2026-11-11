# Entrega Final

Esta carpeta reúne la planificación y las referencias propias de la presentación final de METALFITNESS, prevista para el 11 o el 13 de noviembre de 2026.

## Documentos

| Archivo | Propósito |
|---|---|
| [PLAN_ENTREGA_FINAL.md](PLAN_ENTREGA_FINAL.md) | Etapas, dependencias, criterios de aceptación y seguimiento del trabajo restante |
| [DISENO_FRONTEND.md](DISENO_FRONTEND.md) | Dirección estética del sitio público y de la aplicación autenticada |
| [ETAPA_0_LINEA_BASE.md](ETAPA_0_LINEA_BASE.md) | Línea base, inventario, recorrido principal y matriz de requisitos (etapa 0) |
| [ETAPA_2_IDENTIDAD.md](ETAPA_2_IDENTIDAD.md) | Login, sesión y rutas protegidas por rol; resumen de la seguridad ya implementada (etapa 2) |
| [ETAPA_1_CONTRATOS_RECORRIDOS.md](ETAPA_1_CONTRATOS_RECORRIDOS.md) | Prioridades, rutas, pantallas con sus contratos, wireflows, datos de demostración y decisiones del equipo (etapa 1) |
| [ETAPA_3_MEMBERS.md](ETAPA_3_MEMBERS.md) | Padrón de personas, asignaciones, vigencia para Booking y pantallas de administración; lista de pruebas pendientes con Docker (etapa 3) |
| [ETAPA_4_BOOKING.md](ETAPA_4_BOOKING.md) | Actividades, clases, reservas, ingreso por DNI y ausencias; invariantes en la base y pruebas pendientes con Docker (etapa 4) |
| [ETAPA_5_MENSAJERIA.md](ETAPA_5_MENSAJERIA.md) | Relay, notificaciones durables, reintentos, DLQ y validación local aprobada (etapa 5) |
| [ETAPA_6_BOOKING_BENEFITS.md](ETAPA_6_BOOKING_BENEFITS.md) | Asistencias/ausencias al ledger, historial propio y recuperación de Benefits; 32 escenarios E2E aprobados |
| [ETAPA_7_CLUB.md](ETAPA_7_CLUB.md) | Integraciones, vinculación, cuenta para el administrador y reversiones idempotentes; 38 escenarios E2E aprobados |
| [ETAPA_8_BUSQUEDA.md](ETAPA_8_BUSQUEDA.md) | Búsqueda de clases en OpenSearch: indexer por eventos, reindexado con alias, retraso medido y degradación; 44 escenarios E2E aprobados |
| [ETAPA_9_CACHE.md](ETAPA_9_CACHE.md) | Caché del catálogo de actividades en Redis elegida y medida con la misma carga; corrección del pool del gateway; 24 escenarios E2E |
| [ETAPA_10_TRAINING.md](ETAPA_10_TRAINING.md) | Planes de entrenamiento y alimenticios, mediciones inmutables y consultas nutricionales con MongoDB; autorización por asignación; 27 escenarios E2E |
| [ETAPA_11_FRONTEND.md](ETAPA_11_FRONTEND.md) | Identidad visual en todas las pantallas, firma en movimiento, móvil y validación de los cuatro roles desde la interfaz |
| [ETAPA_13_MELT.md](ETAPA_13_MELT.md) | Manual recibido de MELT, APIs, reglas del referido, pendientes y pruebas previstas; implementación reservada para la etapa 13 |

## Fuentes de verdad

La documentación se interpreta en este orden:

1. [`SPEC.md`](../../SPEC.md) define alcance funcional, actores, permisos y reglas de negocio.
2. [`ARCHITECTURE.md`](../ARCHITECTURE.md), los [ADR](../adr/) y los [contratos](../contracts/) definen backend, servicios e integraciones.
3. `DISENO_FRONTEND.md` define solamente la presentación visual y la experiencia de uso.

El brief de diseño no modifica reglas de negocio ni decisiones arquitectónicas. Si propone planes, precios, horarios, descuentos, funcionalidades o datos distintos de las fuentes anteriores, esa parte se descarta.

## Estado inicial

- Entregas 1 y 2 integradas en `main`.
- Capacidad propia de Benefits, persistencia, gateway, primera traza, resiliencia y pruebas de carga ya validadas para la Entrega 2.
- Integración con la capacidad externa pendiente de que la cátedra confirme proveedor y contrato.
- Frontend actual limitado al estado del sistema y a la cuenta del Club (login, activación, saldo y membresías); el sitio y las demás pantallas funcionales todavía deben desarrollarse.
- Club ampliado (registro externo, membresías e identidad) publicado en Vercel el 08/11/2026 (D-75). Estado detallado al 09/11/2026 en [ETAPA_0_LINEA_BASE.md](ETAPA_0_LINEA_BASE.md).

