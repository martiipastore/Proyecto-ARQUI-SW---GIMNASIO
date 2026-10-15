# Plan de trabajo — Entrega 2

> **Entrega:** viernes 23 de octubre de 2026  
> **Objetivo:** disponer de un flujo del dominio propio funcionando de punta a punta, con persistencia real, logs correlacionados, una primera traza distribuida y las decisiones arquitectónicas exigidas para la Entrega 2.  
> **Estado inicial:** la Entrega 1 dejó un esqueleto ejecutable, contratos y decisiones de diseño. Los servicios todavía no implementan funcionalidades de negocio.

## 1. Propósito de este documento

Este documento organiza el trabajo de la Entrega 2 en etapas pequeñas y verificables. Cada etapa define:

- qué se debe implementar o documentar;
- qué requisitos del SPEC y de la entrega cubre;
- qué decisiones arquitectónicas afecta;
- qué evidencia permite considerarla terminada;
- qué commits conviene realizar.

El documento debe actualizarse durante el desarrollo. Una etapa solo se marca como completada cuando su criterio de aceptación y su evidencia están disponibles.

### Estados

- **Pendiente:** todavía no comenzó.
- **En curso:** existe trabajo iniciado, pero falta evidencia.
- **Bloqueada:** depende de una definición o recurso externo.
- **Completada:** cumple todos sus criterios de aceptación.

## 2. Resultado esperado de la Entrega 2

Al finalizar estas etapas se deberá poder demostrar:

1. Al menos un servicio con una funcionalidad real de negocio.
2. La capacidad propia publicada funcionando mediante su contrato.
3. Un almacenamiento real integrado al flujo.
4. Logs JSON estructurados y correlacionados.
5. Una traza distribuida del flujo principal.
6. Las decisiones D2, D6, D7, D9, D10, D12 y D13.
7. La validación de D1, D3 y D5.
8. Una primera versión de D11.
9. D8 actualizada únicamente si cambia el contrato propio.
10. Tests automáticos y evidencia reproducible de la demostración.

## 3. Alcance funcional elegido

### 3.1 Servicio principal

El servicio propuesto para esta entrega es `benefits-service`, porque:

- representa la capacidad propia publicada a otros grupos;
- ya cuenta con un contrato OpenAPI y un mock ejecutable;
- usa arquitectura hexagonal, permitiendo demostrar D2 con código real;
- necesita PostgreSQL y garantías de consistencia;
- permite demostrar idempotencia frente a reintentos;
- ya tiene routing previsto en el API Gateway.

### 3.2 Flujo mínimo de punta a punta

El flujo mínimo demostrable será:

```text
Partner → API Gateway → benefits-service → PostgreSQL
```

Operaciones mínimas:

1. Acreditar puntos mediante `POST /v1/accounts/{externalUserId}/credits`.
2. Consultar el saldo mediante `GET /v1/accounts/{externalUserId}/balance`.
3. Consultar movimientos paginados mediante `GET /v1/accounts/{externalUserId}/movements`.
4. Repetir la acreditación con la misma `Idempotency-Key` y comprobar que no se duplica el movimiento.

Este recorte cubre HU-29, HU-32, CU-06, RN-22, RN-26, RN-27 y los casos límite CL-08, CL-08b y CL-08c. Débitos, canjes y administración completa del catálogo pueden incorporarse después del flujo mínimo, sin debilitar el contrato ya publicado.

### 3.3 Datos necesarios para la demostración

El entorno de desarrollo y los tests deberán disponer de:

- un partner activo;
- una API key válida, almacenada de forma segura según ADR-006;
- una cuenta de beneficios;
- una vinculación entre el partner y el identificador externo de la cuenta.

Los datos de demostración deben cargarse mediante migraciones, fixtures de tests o un mecanismo de inicialización documentado. No se deben agregar endpoints ocultos ni credenciales reales al repositorio.

### 3.4 Interpretación alternativa de “además”

Si la cátedra confirma que “un servicio operativo, además de la funcionalidad a compartir” exige dos capacidades separadas, se agregará un flujo mínimo en `members-service`. Esa ampliación no debe retrasar el flujo principal de Benefits y se registrará como cambio de alcance en este documento.

## 4. Principios que se mantienen

Durante toda la Entrega 2 se deben respetar estas decisiones ya aceptadas:

- El SPEC es la fuente de verdad del negocio.
- `benefits-service` conserva arquitectura hexagonal.
- Cada servicio accede exclusivamente a su propia base.
- Las reglas viven en dominio o aplicación, nunca en handlers o adaptadores.
- Los errores HTTP siguen RFC 7807 mediante `pkg/problem`.
- Las escrituras del contrato de partners requieren `Idempotency-Key`.
- El saldo no puede ser negativo y los movimientos son inmutables.
- Los logs no incluyen emails, DNI, tokens, API keys ni cuerpos completos.
- Todo cambio arquitectónico requiere actualizar o crear el ADR correspondiente.

## 5. Etapas de implementación

## Etapa 0 — Preparación y línea base

**Estado:** Pendiente

### Objetivo

Comenzar desde una base conocida, reproducible y sin cambios pendientes.

### Tareas

- Crear una rama siguiendo GitHub Flow, por ejemplo `feature/etapa-02-dominio-e2e`.
- Verificar que `main` esté sincronizada con `origin/main`.
- Ejecutar los tests existentes.
- Levantar el stack actual y verificar los health checks.
- Registrar cualquier falla previa al desarrollo.
- Confirmar que el contrato de partners pasa `make contract-lint`.
- Actualizar la tabla de seguimiento de este documento con la fecha de inicio.

### Criterio de aceptación

- Repositorio limpio.
- Tests de línea base ejecutados.
- Stack inicial levantable.
- Contrato actual validado.
- Fallos preexistentes documentados.

### Evidencia

- Salida resumida de tests.
- Estado de contenedores.
- Resultado del lint del contrato.

### Commit sugerido

No necesita commit si no se modifica ningún archivo. Si se registra la línea base en este documento:

`docs: registrar línea base de la entrega 2`

---

## Etapa 1 — Revisión y cierre inicial de decisiones

**Estado:** Pendiente

### Objetivo

Alinear las decisiones obligatorias antes de que el código las vuelva implícitas.

### Tareas

#### Validar decisiones existentes

- **D1:** confirmar que Benefits conserva la responsabilidad sobre cuentas, saldo, movimientos, catálogo, canjes y partners.
- **D3:** confirmar PostgreSQL para Benefits y documentar los patrones de acceso del flujo elegido.
- **D5:** confirmar la comunicación Partner → Gateway → Benefits y sus timeouts, errores y reintentos.
- **D2:** revisar que el diseño del caso de uso respete dominio, aplicación, puertos y adaptadores.
- **D7:** confirmar que el flujo crítico de saldo e idempotencia no depende de una caché.
- **D8:** comparar el OpenAPI con el flujo mínimo. Solo cambiarlo si existe una incompatibilidad real.

#### Crear o completar decisiones obligatorias

- **D6 — Búsqueda:** OpenSearch, entidad `Clase`, filtros, paginación, ordenamiento, evento `clase.actualizada` y retraso objetivo menor a 2 segundos.
- **D9 — Consumo del proveedor:** capacidad externa, contrato, adaptador, autenticación, errores, cuotas, costos y comportamiento ante indisponibilidad.
- **D10 — Resiliencia:** dependencia protegida, timeout, reintentos, circuit breaker o degradación elegida y forma de comprobarla.
- **D11 — Observabilidad v1:** logs, correlación, trazas, métricas iniciales y herramientas.
- **D12 — Balanceo:** `booking-service` con dos instancias detrás de Traefik, health checks y evidencia futura de distribución.
- **D13 — Capacidad y costos:** hipótesis inicial, flujo a medir, umbrales y alternativas. Los resultados se completan en la Etapa 7.

### Criterio de aceptación

- Todas las decisiones obligatorias tienen un archivo identificable.
- No hay contradicciones entre SPEC, ADR, ARCHITECTURE y contrato.
- Las decisiones que aún no tienen evidencia están marcadas como iniciales, no como implementadas.

### Evidencia

- Índice de ADR actualizado.
- Enlaces desde `docs/ARCHITECTURE.md`.
- Estado de cada decisión: propuesta, aceptada o reemplazada.

### Commits sugeridos

- `docs: validar decisiones base de la entrega 2`
- `docs: registrar decisiones D6 y D9 a D13`

---

## Etapa 2 — Dominio y persistencia de Benefits

**Estado:** Pendiente

### Objetivo

Implementar las reglas mínimas de cuentas, saldo, movimientos e idempotencia con PostgreSQL real.

### Tareas

#### Dominio

- Modelar los conceptos necesarios sin tags de transporte o persistencia.
- Representar movimientos inmutables.
- Proteger la regla de saldo como suma de movimientos.
- Diferenciar la identidad interna de una cuenta del `externalUserId` usado por cada partner.
- Definir errores de dominio para cuenta no vinculada, conflicto de idempotencia y operación inválida.

#### Aplicación

- Implementar los casos de uso de acreditar puntos, consultar saldo y listar movimientos.
- Definir puertos de entrada para esos casos.
- Definir puertos de salida para cuentas, movimientos, partners e idempotencia.
- Mantener la lógica independiente de Gin, pgx y HTTP.

#### PostgreSQL

- Crear migraciones versionadas.
- Crear tablas y restricciones para cuentas, partners, vinculaciones, movimientos e idempotencia.
- Garantizar unicidad de la idempotencia por partner.
- Conservar la respuesta original de la primera ejecución durante al menos 30 días.
- Ejecutar el movimiento y el registro de idempotencia dentro de la misma transacción.
- Agregar índices para las consultas por cuenta, partner, fecha y paginación.
- Mantener permisos exclusivos sobre `benefits_db`.

#### Concurrencia

- Evitar que dos requests con la misma clave creen dos movimientos.
- Resolver carreras sin depender solamente de una verificación previa en Go.
- Devolver conflicto si la misma clave se reutiliza con datos diferentes.

### Criterio de aceptación

- Una acreditación confirmada produce exactamente un movimiento.
- Repetir la misma operación devuelve el resultado original.
- Reutilizar la clave con otro payload devuelve el error definido.
- Saldo y movimientos se recuperan desde PostgreSQL.
- Los tests de arquitectura continúan pasando.

### Evidencia

- Tests unitarios de RN-22, RN-26 y RN-27.
- Tests de integración con PostgreSQL real mediante Testcontainers.
- Prueba concurrente de idempotencia.
- Migraciones ejecutables desde una base vacía.

### Commits sugeridos

- `feat(benefits): modelar cuentas y movimientos de puntos`
- `feat(benefits): agregar persistencia PostgreSQL e idempotencia`
- `test(benefits): verificar reglas y concurrencia del ledger`

---

## Etapa 3 — API publicada e integración con el Gateway

**Estado:** Pendiente

### Objetivo

Exponer el flujo de Benefits respetando el contrato versionado y hacerlo accesible a través del Gateway.

### Tareas

- Implementar handlers como adaptadores de entrada.
- Validar path, headers, body y parámetros de paginación.
- Autenticar al partner mediante `X-API-Key` según ADR-006 y D8.
- Aplicar aislamiento por partner según RN-26.
- Exigir `Idempotency-Key` en escrituras.
- Traducir errores de dominio a RFC 7807.
- Incluir `correlation_id` en errores.
- Verificar el mapeo entre el prefijo público del Gateway y las rutas `/v1` del contrato.
- Asegurar que el Gateway no contenga reglas de saldo, idempotencia o partners.
- Mantener el mock de Prism para validar consumidores; no usarlo como reemplazo de la implementación real.
- Ejecutar `make contract-lint`.
- Agregar tests que comparen los comportamientos principales con el contrato.

### Criterio de aceptación

Desde un cliente externo se puede:

1. acreditar puntos pasando por el Gateway;
2. consultar el saldo resultante;
3. listar el movimiento;
4. repetir la acreditación sin duplicarla;
5. obtener errores contractuales ante API key inválida, cuenta no vinculada o idempotencia conflictiva.

### Evidencia

- Requests y respuestas reproducibles.
- Tests de handlers.
- Tests de integración Gateway → Benefits.
- Contrato sin errores ni advertencias.

### Commits sugeridos

- `feat(benefits): exponer API de partners versionada`
- `feat(gateway): integrar capacidad pública de beneficios`
- `test(contracts): verificar flujo de partners contra el contrato`

---

## Etapa 4 — Logs correlacionados y primera traza distribuida

**Estado:** Pendiente

### Objetivo

Poder reconstruir una acreditación sin inspeccionar manualmente cada proceso.

### Tareas

#### Logs

- Confirmar salida JSON en Gateway y Benefits.
- Propagar `X-Correlation-ID` del Gateway al servicio.
- Incluir `service`, `level`, `msg`, `correlation_id`, `trace_id` y `span_id`.
- Registrar el hecho de negocio sin incluir API keys, identificadores sensibles ni payloads completos.
- Registrar errores de dependencias con información suficiente para diagnóstico.

#### Trazas

- Incorporar OpenTelemetry en Gateway y Benefits.
- Instrumentar HTTP entrante y saliente.
- Instrumentar la operación PostgreSQL o agregar spans manuales alrededor del repositorio.
- Propagar `traceparent`.
- Incorporar OTel Collector y Jaeger al entorno necesario para la demostración.
- Nombrar spans por operación de negocio y no por datos sensibles.

#### D11

- Documentar objetivo, componentes, señales y límites de la primera versión.
- Diferenciar correlación por logs de trazabilidad distribuida.
- Registrar qué métricas quedan para etapas posteriores.

### Criterio de aceptación

Una acreditación genera:

- logs correlacionados en Gateway y Benefits;
- un mismo `trace_id` a través del flujo;
- una traza visible con spans del Gateway, Benefits y persistencia;
- ausencia de secretos y datos personales en logs y atributos.

### Evidencia

- Captura o exportación de una traza.
- Ejemplos sanitizados de logs JSON.
- Test de propagación de contexto.
- ADR D11 actualizado con lo realmente implementado.

### Commits sugeridos

- `feat(observability): propagar contexto de trazas`
- `feat(observability): instrumentar gateway y benefits`
- `docs: registrar primera versión de observabilidad`

---

## Etapa 5 — Consumo de una capacidad externa y D9

**Estado:** Bloqueada hasta conocer el proveedor

### Objetivo

Documentar e integrar la capacidad ofrecida por otro grupo sin acoplar el dominio a su contrato.

### Tareas

- Obtener el OpenAPI o contrato equivalente del proveedor.
- Identificar la funcionalidad concreta que aporta al sistema.
- Definir el servicio consumidor y el caso de uso.
- Registrar autenticación, timeout, reintentos, cuotas y errores.
- Definir un puerto con vocabulario propio del dominio.
- Implementar un adaptador que traduzca el contrato externo.
- Preparar un fake o mock contractual para trabajar cuando el proveedor no esté disponible.
- Definir degradación o fallo cerrado según el impacto de la capacidad.
- Registrar compatibilidad y estrategia frente a cambios del proveedor.
- Completar D9 con evidencia real.

### Criterio de aceptación

- Existe una razón de negocio clara para consumir la capacidad.
- El contrato externo no se filtra al dominio.
- La indisponibilidad tiene un comportamiento definido.
- El flujo puede probarse con el proveedor o con un mock fiel al contrato.
- D9 identifica proveedor, consumidor, adaptación y riesgos.

### Evidencia

- Contrato recibido.
- Test del adaptador.
- Ejemplo de respuesta normal y de error.
- ADR D9 aceptado.

### Commit sugerido

`feat(integration): consumir capacidad externa mediante adaptador`

---

## Etapa 6 — Resiliencia y caída controlada

**Estado:** Pendiente

### Objetivo

Demostrar que el flujo principal responde de forma controlada cuando una dependencia falla o se vuelve lenta.

### Tareas

- Elegir una caída relevante para la entrega, preferentemente Benefits detenido o PostgreSQL no disponible.
- Confirmar el timeout extremo a extremo.
- Evitar reintentos automáticos en el Gateway.
- Permitir que el partner reintente una escritura con la misma `Idempotency-Key`.
- Diferenciar:
  - servicio inaccesible → `502`;
  - timeout → `504`;
  - dependencia interna no disponible → `503`.
- Verificar que un timeout no duplique movimientos.
- Registrar la falla mediante logs y trazas.
- Definir recuperación y reconciliación.
- Completar D10 con el mecanismo implementado.
- Crear una primera versión de `docs/POSTMORTEM.md` después del ejercicio.

### Criterio de aceptación

- La caída puede reproducirse.
- El sistema corta dentro del tiempo definido.
- El cliente recibe un error contractual.
- Reintentar con la misma clave no duplica puntos.
- La evidencia permite reconstruir la línea temporal.
- El servicio vuelve a operar después de restaurar la dependencia.

### Evidencia

- Comandos de la prueba.
- Tiempos observados.
- Logs y traza asociados.
- Estado de la base antes y después.
- `POSTMORTEM.md`.

### Commits sugeridos

- `feat(resilience): proteger el flujo de beneficios ante fallas`
- `test(resilience): cubrir timeout y reintento idempotente`
- `docs: registrar postmortem de la caída controlada`

---

## Etapa 7 — Tests de carga, capacidad y costos

**Estado:** Pendiente

### Objetivo

Obtener evidencia inicial para D13 y detectar el principal límite del flujo.

### Tareas

- Crear scripts k6 en `tests/load/`.
- Separar escenarios de lectura y escritura.
- Incluir:
  - acreditaciones con claves únicas;
  - reintentos con la misma clave;
  - consultas de saldo;
  - movimientos paginados.
- Definir volumen, duración, concurrencia y datos iniciales.
- Medir latencia p50, p95 y p99, throughput y tasa de error.
- Observar conexiones PostgreSQL, CPU y memoria.
- Identificar el primer cuello de botella.
- Comparar al menos dos alternativas de mejora.
- Estimar recursos y costo del despliegue propuesto.
- Completar D13 con resultados, límites y supuestos.
- No presentar cifras sin fecha, ambiente y configuración.

### Umbrales iniciales

Los valores definitivos se validarán con la prueba. Como punto de partida:

- tasa de error funcional: 0 %;
- ninguna duplicación por idempotencia;
- p95 de lecturas menor a 200 ms;
- p95 de escrituras menor a 500 ms.

### Criterio de aceptación

- Los scripts son reproducibles.
- Los resultados están guardados y analizados.
- Se identifica un límite concreto.
- D13 relaciona capacidad, alternativas y costos.
- Las conclusiones distinguen medición de estimación.

### Evidencia

- Scripts k6.
- Resultados con fecha y entorno.
- Gráficos o tablas resumidas.
- ADR D13 completado.

### Commits sugeridos

- `test(load): agregar escenarios de carga de beneficios`
- `docs: documentar capacidad y costos del flujo principal`

---

## Etapa 8 — Revisión documental y preparación de la entrega

**Estado:** Pendiente

### Objetivo

Asegurar que código, contratos, decisiones y demostración describan el mismo sistema.

### Tareas

- Actualizar el estado de implementación del README.
- Actualizar `docs/ARCHITECTURE.md` con los componentes realmente incorporados.
- Revisar D1, D2, D3, D5, D7 y D8.
- Confirmar que D6, D9, D10, D11, D12 y D13 estén enlazadas.
- Actualizar el CHANGELOG del contrato solo si cambió.
- Ejecutar tests, lint y validación del contrato.
- Levantar el stack desde cero.
- Ejecutar el flujo principal.
- Ejecutar la caída controlada.
- Preparar un guion corto de demostración.
- Completar la tabla de seguimiento con commits y evidencias.

### Criterio de aceptación

- Un integrante puede reproducir la demostración siguiendo el README.
- La documentación distingue diseño objetivo de implementación real.
- No hay ADR obligatorios ausentes.
- No hay secretos ni datos personales en repositorio o logs.
- Todos los checks requeridos pasan.

### Commit sugerido

`docs: preparar evidencia de la entrega 2`

## 6. Seguimiento

| Etapa | Estado | Evidencia esperada | Commit o PR |
|---|---|---|---|
| 0. Preparación y línea base | Pendiente | Tests, stack y contrato validados | — |
| 1. Decisiones iniciales | Pendiente | ADR obligatorios identificados | — |
| 2. Dominio y persistencia | Pendiente | Migraciones y tests de Benefits | — |
| 3. API y Gateway | Pendiente | Flujo contractual reproducible | — |
| 4. Observabilidad | Pendiente | Logs correlacionados y traza | — |
| 5. Capacidad externa | Bloqueada | Contrato y adaptador del proveedor | — |
| 6. Resiliencia | Pendiente | Caída controlada y postmortem | — |
| 7. Capacidad y costos | Pendiente | k6, resultados y D13 | — |
| 8. Revisión final | Pendiente | Checklist y guion de demostración | — |

## 7. Matriz de decisiones de la Entrega 2

| Decisión | Acción | Momento de cierre |
|---|---|---|
| D1 — Límites de servicios | Validar | Etapa 1 y revisión en Etapa 8 |
| D2 — Arquitectura interna | Validar con Benefits real | Etapas 1 y 2 |
| D3 — Persistencia | Validar PostgreSQL y patrones de acceso | Etapas 1 y 2 |
| D5 — Comunicación | Validar timeouts, errores y reintentos | Etapas 1, 3 y 6 |
| D6 — Búsqueda | Crear ADR | Etapa 1 |
| D7 — Caché | Revisar y presentar | Etapa 1 |
| D8 — Contrato propio | Actualizar solo si cambia | Etapas 1 y 3 |
| D9 — Consumo del proveedor | Crear e implementar adaptación | Etapa 5 |
| D10 — Resiliencia | Crear y validar con caída | Etapa 6 |
| D11 — Observabilidad | Crear primera versión | Etapa 4 |
| D12 — Balanceo | Crear ADR | Etapa 1 |
| D13 — Capacidad y costos | Iniciar y completar con mediciones | Etapas 1 y 7 |

## 8. Dependencias y riesgos

| Riesgo | Impacto | Tratamiento |
|---|---|---|
| No recibir a tiempo el contrato del otro grupo | D9 incompleta | Solicitarlo temprano y preparar el puerto; usar mock contractual hasta disponer del proveedor |
| Implementar demasiado alcance funcional | Retraso de la entrega | Cerrar primero acreditación, saldo, movimientos e idempotencia |
| Modificar el OpenAPI sin necesidad | Ruptura para consumidores | Implementar contra D8 y cambiar el contrato solo mediante versionado |
| Mezclar dominio con Gin o pgx | Incumplimiento de D2 | Mantener tests de arquitectura y revisar dependencias |
| Idempotencia solo en memoria | Duplicación al reiniciar o escalar | Persistir clave, parámetros y resultado en la transacción |
| Logs sin contexto común | No se puede reconstruir el flujo | Propagar correlation ID y trace context desde el Gateway |
| Agregar observabilidad demasiado amplia | Pérdida de foco | Instrumentar primero el flujo principal; ampliar después |
| Pruebas de carga sin ambiente controlado | Resultados no comparables | Registrar hardware, configuración, datos, duración y versiones |

## 9. Fuera del alcance mínimo de la Entrega 2

Salvo que una evidencia o decisión obligatoria lo requiera, no es necesario completar todavía:

- todo el frontend funcional;
- todos los casos de uso de los cuatro servicios;
- implementación completa de búsqueda con OpenSearch;
- implementación completa de caché;
- balanceo real de Booking;
- todos los eventos y consumidores asíncronos;
- tableros definitivos de observabilidad;
- despliegue productivo final.

Estos elementos siguen siendo parte del trabajo final. En la Entrega 2 deben quedar decisiones coherentes y una base compatible con su implementación posterior.

## 10. Checklist final de la Entrega 2

- [ ] Existe al menos un servicio con lógica de negocio operativa.
- [ ] La capacidad propia se consume mediante su contrato.
- [ ] PostgreSQL participa en el flujo principal.
- [ ] La idempotencia evita duplicaciones.
- [ ] Los logs son JSON y están correlacionados.
- [ ] Existe una traza distribuida visible.
- [ ] D1 fue validada.
- [ ] D2 está respaldada por código real.
- [ ] D3 fue validada.
- [ ] D5 fue validada.
- [ ] D6 está documentada.
- [ ] D7 está revisada.
- [ ] D8 fue revisada y solo cambió si era necesario.
- [ ] D9 está documentada con el proveedor real.
- [ ] D10 está documentada y demostrada.
- [ ] D11 tiene una primera versión.
- [ ] D12 está documentada.
- [ ] D13 contiene mediciones y estimaciones.
- [ ] Los tests unitarios e integración pasan.
- [ ] El contrato pasa Spectral.
- [ ] La caída controlada está registrada.
- [ ] README y ARCHITECTURE reflejan el estado real.
- [ ] La demostración puede reproducirse desde un entorno limpio.

