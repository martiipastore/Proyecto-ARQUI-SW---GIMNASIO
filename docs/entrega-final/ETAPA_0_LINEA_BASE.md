# Etapa 0 — Línea base y trazabilidad

Fecha: 9 de noviembre de 2026. Base: `main` en `397cc3c` (PR #14). Rama de trabajo:
`feature/entrega-final-etapa-00-linea-base`. Plan: [PLAN_ENTREGA_FINAL.md](PLAN_ENTREGA_FINAL.md).

**Objetivo:** partir de un `main` estable y convertir los requisitos de la presentación
final y de la defensa individual en una matriz verificable, sin requisitos huérfanos.

**Estado:** documentación y herramienta completas. La ejecución completa de la línea
base (integraciones con Docker, Spectral, frontend y stack limpio) queda a cargo de
otro integrante, porque en el equipo donde se preparó la etapa el disco estaba lleno
y Docker no podía iniciar. Ver §2.3.

## 1. Cómo reproducir la línea base

Un solo comando, desde la raíz del repositorio, con Docker Desktop iniciado:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/linea-base.ps1
```

El script ([scripts/linea-base.ps1](../../scripts/linea-base.ps1)) ejecuta cada chequeo
aunque otro falle y guarda el resultado en
`docs/evidence/entrega-final/etapa0/linea-base.json`. No modifica código, no usa `.env`
para el E2E y no borra contenedores ni volúmenes.

| Grupo | Chequeos |
|---|---|
| Go | `go vet` y `go test -count=1` en los 7 módulos; con Docker, las integraciones son obligatorias (`REQUIRE_DOCKER_TESTS=1`) |
| Herramientas de prueba | Inyector de resiliencia (tests y vet), diagnóstico de pool y ejecutor E2E (vet) |
| Frontend | `npm ci` y `npm run build` (TypeScript + Vite) |
| Contratos y despliegue local | Spectral sobre los contratos v2 y v1; `docker compose config` |
| E2E | 14 escenarios de identidad, registro externo y membresías con servicios, PostgreSQL y Mailpit reales |
| Producción | Lecturas sin credenciales: frontend 200, partner sin API key 401, API sin JWT 401, rutas `internal` 404 |

Opciones: `-SinRyuk` si Testcontainers no logra iniciar su contenedor de limpieza (pasó
en Docker Desktop recién iniciado), `-OmitirE2E` y `-OmitirProduccion`.

El stack completo desde cero se verifica aparte, porque reemplaza temporalmente el entorno
local: `cp .env.example .env && make up` y `GET http://localhost:8080/api/v1/status` debe
responder `ready`. La revisión aislada de D-61 (`tests/e2e/review-delivery.ps1`) también
sirve, pero reescribe la evidencia de la etapa 8 de la Entrega 2.

## 2. Resultado de la línea base

### 2.1 Ejecutado el 09/11/2026

| Chequeo | Resultado |
|---|---|
| `go vet` en los 7 módulos | Correcto |
| Tests unitarios de los 7 módulos | Correcto; integraciones salteadas por falta de Docker |
| Inyector de resiliencia, diagnóstico de pool y ejecutor E2E | Correcto |
| Producción: `/` | 200 |
| Producción: `/partner-api/v2/accounts/linea-base/balance` sin API key | 401 en formato de problema |
| Producción: `/api/v1/users/me` sin JWT | 401 |
| Producción: `/api/v1/internal/linea-base` | 404 |
| Sintaxis del script de línea base | Correcta (parser de PowerShell) |

El mismo día, antes de quedarse sin disco, se ejecutaron contra PostgreSQL real todas
las integraciones de Members y Benefits y el E2E de 14 escenarios (PR #14,
[cb04-v2-e2e.json](../evidence/club/cb04-v2-e2e.json)).

### 2.2 Hallazgos

| Hallazgo | Impacto | Acción |
|---|---|---|
| Producción todavía no tiene el PR #14: los errores de partners informan `instance: /v2/...` | Diferencia menor con el contrato | Publicar con `vercel deploy --prod` desde `main` (D-75) |
| GitHub Actions bloqueado por facturación (L-23) | Los PR no tienen checks automáticos | Correr `scripts/linea-base.ps1` antes de integrar |
| Testcontainers falla al iniciar su contenedor Ryuk con Docker recién iniciado | Falsos negativos en integraciones | Repetir con `-SinRyuk` |
| `golangci-lint` no está instalado en el equipo usado | Lint completo no ejecutado | Lo ejecuta la CI cuando se desbloquee, o `make lint` con el binario instalado |

### 2.3 Pendiente de ejecutar por otro integrante

- [ ] `scripts/linea-base.ps1` completo con Docker y Node; adjuntar `linea-base.json`.
- [ ] `make up` desde cero y `/api/v1/status` en `ready`.
- [ ] Publicar `main` en Vercel y repetir la verificación de producción.

## 3. Inventario: implementado, diseñado y pendiente

| Componente | Implementado | Diseñado (ADR) sin implementar | Pendiente |
|---|---|---|---|
| Gateway | Routing, correlación, timeouts, errores 502/504, JWT RS256, inyección de identidad, autorización por rol, bloqueo de `internal` | Rate limiting con Redis (§5.1) | Control de tráfico (etapa 12) |
| Members (Capas, PostgreSQL) | Login, roles, activación por correo, registro externo, membresías con historial y outbox | Alumnos con DNI, profesionales, asignaciones, bajas lógicas | Etapa 3 |
| Benefits (Hexagonal, PostgreSQL) | Ledger, API v2 de partners, registro externo, límites por integración, idempotencia | Acreditación por asistencia | Etapa 6 |
| Booking (Hexagonal + CQRS) | Solo esqueleto y health check | Clases, reservas, cupo, ingreso (ADR-002 v2, ADR-005 v2) | Etapa 4 |
| Booking indexer / OpenSearch | Esqueleto | Búsqueda (ADR-014) | Etapa 8 |
| Training (Capas, MongoDB) | Esqueleto y conexión a MongoDB | — | Etapa 10 |
| Notification worker (MongoDB) | Esqueleto y almacenamiento | Consumo de eventos y SMTP | Etapa 5 |
| Mensajería (RabbitMQ) | Contenedor en Compose; outbox de Members sin publicador | Topología, reintentos y DLQ (ARCHITECTURE §8) | Etapa 5 |
| Caché (Redis) | Contenedor en Compose | ADR-007 | Etapa 9 |
| Balanceo (Traefik) | — | ADR-012 v2 | Etapa 12 |
| Observabilidad | Logs JSON correlacionados; trazas Gateway → Benefits → PostgreSQL | Métricas, tableros y alertas (ADR-011 v2) | Etapa 14 |
| Frontend | Login, activación, saldo y membresías propios; estado del sistema | Diseño visual (DISENO_FRONTEND.md) | Etapa 11 |
| Nube | Vercel + PostgreSQL gestionado (ADR-021) | — | Publicar `main` |
| Capacidad externa (D9) | — | ADR-009 propuesto | Bloqueada: sin proveedor |

## 4. Recorrido principal

Se mantiene el recorrido de [PLAN_ENTREGA_FINAL §4](PLAN_ENTREGA_FINAL.md#4-recorrido-principal-propuesto)
con un ajuste por D-76 (sin canjes), **confirmado por el equipo el 09/11/2026 (D-85)**:

1. El administrador da de alta al alumno y le asigna una membresía. *(Members, etapa 3)*
2. La membresía se persiste y produce una notificación asíncrona. *(Members → RabbitMQ → Notification, etapa 5)*
3. El alumno inicia sesión y busca una clase. *(Gateway + Booking + OpenSearch, etapas 2, 4 y 8)*
4. El alumno reserva; Booking valida identidad, membresía, superposición y cupo. *(etapa 4)*
5. El alumno registra su ingreso con DNI. *(etapa 4)*
6. Booking confirma la asistencia y publica el evento. *(etapas 4 y 6)*
7. Benefits consume el evento y acredita 500 puntos exactamente una vez. *(etapa 6)*
8. **Cambio:** el alumno consulta saldo y movimientos; un sistema de otro grupo acredita
   puntos por la API v2. El canje del recorrido original se descartó (D-76).
9. La capacidad externa se incorpora cuando la cátedra asigne el proveedor. *(etapa 13)*

## 5. Funcionalidades mínimas por rol

| Rol | Mínimo para la presentación (SPEC) | Hoy |
|---|---|---|
| Administrador | Alta de alumnos y profesionales, asignaciones, membresías, actividades y clases (HU-01 a HU-08) | Membresías por API, sin pantallas |
| Alumno | Login, su membresía, buscar y reservar clases, cancelar, puntos y movimientos (HU-12 a HU-19) | Login, membresía y saldo |
| Profesor | Sus clases y alumnos, planes de entrenamiento (HU-21, HU-23) | — |
| Nutricionista | Planes alimenticios, mediciones y consultas (HU-24, HU-25, HU-34) | — |
| Ingreso por DNI (kiosco) | Validar DNI y registrar asistencia (HU-22) | — |
| Partner (otro grupo) | Registro, créditos, saldo y movimientos por API v2 (HU-29, HU-32, HU-CB-01) | Implementado y publicado |

## 6. Matriz de trazabilidad

Requisitos del enunciado para la presentación grupal (§6) y la defensa individual,
con su sección de origen. Cada fila apunta a la etapa del plan final que la cierra.

| # | Requisito | Servicio | Pantalla | Prueba | Decisión | Evidencia | Estado | Etapa |
|---|---|---|---|---|---|---|---|---|
| R1 | Frontend completo, del ingreso a la capacidad compartida (§2.2) | web | Todas | E2E desde la web | DISENO_FRONTEND | — | Parcial | 11 |
| R2 | Entrada única con autenticación, routing y control de tráfico (§6) | Gateway | Login | `gateway/internal/server` | ADR-006, §5.1 | Tests de acceso y partners | Parcial: falta control de tráfico | 2, 12 |
| R3 | Tres o más servicios operativos (§2.1) | Members, Benefits, Booking | — | Integraciones por servicio | ADR-001 | CB-02/03/04 | Parcial: 2 de 3 | 3, 4 |
| R4 | Dos patrones internos reconocibles (§2.1) | Benefits (Hexagonal), Members (Capas) | — | `architecture_test.go` | ADR-002 v2 | Tests de arquitectura | Cumplido | — |
| R5 | Capacidad propia desplegada con URL pública (§4.1) | Benefits | — | Producción en `linea-base.ps1` | ADR-008 v4, ADR-021 | §2.1 de este documento | Cumplido; falta publicar PR #14 | 15 |
| R6 | Capacidad del proveedor en el flujo principal (§4) | A definir | A definir | Test de contrato del consumidor | ADR-009 | — | Bloqueado | 13 |
| R7 | Relacional y no relacional integrados al flujo (§2.8) | PostgreSQL (Members, Benefits); MongoDB (Notification, Training) | — | Integraciones | ADR-003 | — | Parcial: MongoDB sin flujo real | 5, 10 |
| R8 | Operación con garantías de consistencia (§2.8) | Benefits (ledger); Booking (cupo) | Saldo; reserva | Concurrencia e idempotencia | ADR-004 v2, ADR-015, ADR-016 | ETAPA_2_BENEFITS, CB-02 | Cumplido en Benefits; cupo pendiente | 4 |
| R9 | Caché con medición antes y después (§2.7) | A elegir | — | k6 con y sin caché | ADR-007 | — | Pendiente | 9 |
| R10 | Evento publicado y consumido, idempotente, con DLQ (§2.5) | Members/Booking → Notification/Benefits | — | Duplicado y mensaje inválido | ADR-005 v2, ARCHITECTURE §8 | — | Pendiente | 5, 6 |
| R11 | Síncrona con timeout, errores y reintentos (§2.5) | Gateway → servicios; Benefits ↔ Members | — | Tests 502/504 y resiliencia | ADR-005 v2, ADR-010 v2, ADR-022 | ETAPA_6_RESILIENCIA | Cumplido | — |
| R12 | Búsqueda con paginación, filtros, orden, actualización y retraso definido (§2.6) | Booking + indexer | Búsqueda de clases | Filtros, orden y lag medido | ADR-014 | — | Pendiente | 8 |
| R13 | Balanceo de 2+ instancias con detección y evidencia (§2.3) | Booking + Traefik | — | Caída de una instancia | ADR-012 v2 | — | Pendiente | 12 |
| R14 | Mecanismo de protección, caída provocada y POSTMORTEM (§2.9) | Gateway/Benefits | — | `verify-resilience.ps1` | ADR-010 v2 | POSTMORTEM.md | Cumplido para Benefits; ampliar | 14 |
| R15 | Logs, métricas, trazas, tablero y alerta (§2.10) | Todos | — | Traza de punta a punta | ADR-011 v2 | ETAPA_4_OBSERVABILIDAD | Parcial: sin métricas ni tablero | 14 |
| R16 | Tests unitarios con cobertura de calidad (§2.11) | Todos | — | `go test -cover` | — | — | Parcial: sin umbral definido | 16 |
| R17 | Integración por servicio con dependencia real (§2.11) | Todos | — | Testcontainers | Reglas del repo | CB-02/03/04 | Parcial: Booking/Training sin casos de uso | 4, 10 |
| R18 | Carga con resultados y análisis (§2.11) | Benefits | — | k6 | ADR-013 v2 | ETAPA_7_CAPACIDAD_COSTOS | Cumplido para Benefits; costos de Oracle desactualizados | 16 |
| R19 | Test de contrato sobre la capacidad consumida (§4) | Consumidor | — | Contrato del proveedor | ADR-009 | — | Bloqueado | 13 |
| R20 | Puesta en marcha única y documentada; secretos fuera del repo (§2.12) | Compose | — | `make up` | README | VALIDACION_ENTREGA_2 | Cumplido; revalidar desde cero | 0 |
| R21 | Repo público, rama principal, ramas, revisiones e integración (§2.12) | — | — | CI | CONTRIBUTING | PR #1 a #14 | Parcial: CI bloqueada | 16 |
| R22 | Documentación y limitaciones actualizadas (§3, §6) | — | — | Revisión | — | README, ARCHITECTURE §15 | Cumplido al 09/11; mantener | 16 |
| R23 | Decisiones D1–D13 verificadas con evidencia (§6) | — | — | — | ARCHITECTURE §16 | Tabla D1–D13 → ADR | Parcial | 16 |
| R24 | Bonus anunciados e integrados (§7) | web, Booking | Ocupación en tiempo real | Playwright | D-40, SPEC §1.6 | — | Anunciado, sin implementar | 16 (el equipo decide si se implementan; no son obligatorios) |

Todos los requisitos tienen etapa asignada. Los únicos sin camino propio son R6 y R19,
que dependen de la asignación del proveedor.

## 7. Desvíos del plan final detectados

| Punto del plan | Situación al 09/11 | Tratamiento |
|---|---|---|
| Etapa 7, Club completo (catálogo, canjes, débitos) | Descartados por el equipo (D-76) | Queda la gestión de partners e integraciones |
| Etapa 15, VM ARM con Traefik | Publicado en Vercel (D-75, ADR-021) | Reemplaza la prueba de compatibilidad ARM de esta etapa por la verificación de producción |
| "CI verde" en los criterios de aceptación | GitHub Actions bloqueado por facturación | `scripts/linea-base.ps1` como verificación previa a integrar |
| Calendario (congelar el 8/11) | Vencido; los repositorios se descargan el 11/11 | El equipo debe priorizar qué etapas entran |
| Diseño del frontend | Existen `DISENO_FRONTEND.md` y `docs/PLAN_DISENO_FRONTEND.md` (FD-01 a FD-05) | Resuelto: manda `DISENO_FRONTEND.md` (D-84) |
