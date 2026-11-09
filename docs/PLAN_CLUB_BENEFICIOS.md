# Plan de corrección integral del Club de Beneficios

> **Decisión de alcance del 08/11/2026:** débitos, catálogo y canjes no se
> implementarán ni se exportarán. Las filas CB-05 a CB-07 conservan propuestas
> anteriores para trazabilidad; no deben leerse como compromisos de la capacidad
> compartida. El contrato operativo es [v2](contracts/benefits-api.v2.yaml).

Actualizado: 9 de noviembre de 2026 (estado de CB-04, publicación en Vercel y
D-75 a D-77). Fuente: pedido del equipo, D-63 a D-77 y
[adenda funcional](SPEC_CLUB_BENEFICIOS.md). No fija una nueva fecha de entrega.

Este plan amplía el producto existente. No reinicia el proyecto ni renumera las
etapas académicas 1–8 o las decisiones D1–D13. D-42 y las evidencias de Entrega 2
conservan su alcance histórico; no certifican los cambios del Club. Un estado
«implementado localmente» no equivale a «completado» ni a «desplegado».

| Etapa | Estado | Trabajo y criterio de cierre | Trazabilidad |
|---|---|---|---|
| CB-01 — Alcance y documentación | En curso: documentos sincronizados, revisión del equipo pendiente | Conciliar SPEC, decisiones Markdown/Word, ADR y etapas; conservar decisiones reemplazadas y distinguir propuesta, implementación y evidencia. Revisar con otro integrante conforme a CONTRIBUTING antes de integrar. | D-63–70; HU-CB-01–08 |
| CB-02 — Integraciones y ledger | Implementación y validación local completadas; revisión del equipo pendiente | Migraciones 001–005 desde base vacía y ledger previo con historial; permisos, revocación, límites, cuotas, eventos repetidos/concurrentes y clientes anteriores comprobados en PostgreSQL real. 53 resultados test/subtest aprobados, cero tests omitidos; vet y Spectral sin hallazgos. Ver CB02_VALIDACION.md. | ADR-016/018 propuestos; D-64/66/73; RN-CB-01–06 |
| CB-03 — Registro, activación y acceso | Implementación y validación local completadas; revisión del equipo pendiente | D-70 corregida en código según ADR-006: JWT RS256 900 s, roles persistidos y bcrypt mínimo 12. Alta/reutilización, prueba de correo, primera contraseña, outbox, login/saldo propio y logout v1 probados. 100 resultados test/subtest y vet aprobados, nueve escenarios E2E con PostgreSQL/SMTP reales locales, build frontend correcto. Ver CB03_VALIDACION.md. | ADR-006 vigente y ADR-017 propuesto; D-63/65/70/74; HU-CB-01/02 |
| CB-04 — Membresías independientes | Implementación y validación local completadas (PR #11); vencimiento según el tipo y solo alumnos agregados el 09/11/2026 (D-77). E2E de 14 escenarios aprobado el 09/11/2026: [cb04-v2-e2e.json](evidence/club/cb04-v2-e2e.json) | Mensual/trimestral/anual/por clases, vigencia, estados e historial en Members. Mantener cuenta y ledger en Benefits sin borrados/reinicios por cambios de membresía. Datos comerciales configurables. Cerrar con vencimiento, retorno y cambio de tipo sin pérdida de puntos/historial. | D-66/67; HU-CB-03; RN-CB-08 |
| CB-05 — Reglas y recompensas | Pendiente | Diseñar ADR antes de implementar. Administrar acciones autorizadas y elegibilidad por integración, tipo/estado, período y restricciones; separar puntos, descuentos, recompensas y membresías promocionales. Probar mes gratuito configurable sin duplicar el otorgamiento ni convertirlo arbitrariamente en puntos. | D-68; HU-CB-04/05; RN-CB-09/10 |
| CB-06 — Canjes y ajustes | Canjes y débitos descartados por el equipo (D-76); los ajustes administrativos quedan para CB-07 | Extender el ledger y sus restricciones juntos para débitos/canjes/ajustes autorizados con justificación, concurrencia e idempotencia; saldo siempre no negativo y auditoría inmutable. No llevar débitos a una tabla paralela que omita el saldo. | D-66; HU-CB-06; RN-CB-02/07 |
| CB-07 — Administración | Pendiente | Autenticación y roles reales antes de exponer gestión. Buscar usuarios, ver origen/puntos/historial/membresía; ajustar puntos; administrar catálogo, promociones, integraciones, permisos, credenciales y límites. Claves visibles solo al emitir/rotar y guardadas como hash. Auditoría de cambios y recuperación de comandos pendientes. | HU-CB-07; RN-CB-03/11; ADR-006 y decisión que lo concilie |
| CB-08 — Regresión, operación y despliegue | En curso: publicado en Vercel y recorrido productivo completado el 08/11/2026 (D-75, ADR-021); faltan los escenarios de CB-05 y la regresión final | Ejecutar diez escenarios integrales, Spectral y bases/correo reales, revisar migraciones/secretos, cuotas, recuperación y limpieza de sesiones/contadores sin purgar ledger/eventos. Inspeccionar Vercel/GitHub con acceso real; documentar antes de cualquier cambio productivo. | HU-CB-08; E2E-CB-01–10; D-69 |

## Evidencia disponible y límites

Actualización CB-03: [validación local](CB03_VALIDACION.md),
[tests y vet](evidence/club/cb03-tests.json) y
[E2E con SMTP](evidence/club/cb03-e2e.json). JWT/roles/bcrypt corregidos frente a
ADR-006; 100 resultados de tests/subtests sin omisiones y nueve escenarios reales
locales aprobados. Esto supera la falta de base/correo de la iteración inicial,
sin acreditar SMTP productivo, automatización de navegador ni membresías/canjes.
La revisión del equipo y la configuración de claves siguen pendientes. Sigue CB-04.

Actualización CB-02: [validación local](CB02_VALIDACION.md) e
[informe de tests](evidence/club/cb02-tests.json). El 06/11/2026 se inició Docker
local y se ejecutó Benefits contra contenedores PostgreSQL aislados: 53 resultados
test/subtest aprobados, cero tests omitidos; `go vet` y Spectral pasaron.
Esto reemplaza la limitación de Docker para CB-02, no valida SMTP, seguridad de
CB-03, diez E2E del Club ni capacidad productiva. La evidencia adicional de CB-03
se registra por separado arriba; no se atribuye a las pruebas de CB-02.

Antecedente de la iteración anterior (conservado para trazabilidad):

En la iteración de identidad pasaron `go test ./...` y `go vet ./...` en Members,
Benefits y Gateway, el build del frontend, sintaxis/referencias YAML de OpenAPI y
validación estática de Compose con `.env.example`. PostgreSQL con testcontainers
se omitió por Docker no disponible; no hay validación de SMTP real, Spectral nuevo,
flujo integral desplegado ni medición de capacidad del registro externo.

Las medidas de etapas 4/6/7 y la evidencia de etapa 8 corresponden al flujo anterior.
No se reutilizan como resultados del nuevo comando, correo o autenticación.
No se declara cumplimiento de todos los escenarios por compilar sus tests.

## Matriz de aceptación integral

| Caso | Escenario | Situación actual |
|---|---|---|
| E2E-CB-01 | Nuevo usuario externo, puntos, bienvenida y activación | Aprobado local: HTTP real, PostgreSQL, SMTP Mailpit y activación; sin navegador/productivo |
| E2E-CB-02 | Usuario existente recibe puntos en su cuenta | Aprobado local: prueba de correo, UUID y contraseña preservados, bcrypt actualizado y saldo correcto |
| E2E-CB-03 | Sin membresía activa conserva puntos | Aprobado local (09/11/2026): cuenta sin membresía con saldo propio; cb04-v2-e2e.json |
| E2E-CB-04 | Membresía vencida y retorno al gimnasio | Aprobado local (09/11/2026): vencida conserva 300 puntos y movimientos; renovación idempotente |
| E2E-CB-05 | Integración no autorizada intenta acreditar | Permisos, estado y revocación comprobados en PostgreSQL/API de Benefits; E2E desplegado pendiente |
| E2E-CB-06 | Evento repetido no acredita dos veces | Aprobado local: distintas keys, simultaneidad y retención en PostgreSQL; evento compartido entre registro y créditos en el E2E |
| E2E-CB-07 | Consulta autenticada de saldo vinculado | Aprobado local: JWT propio y API key del partner; otra integración recibe 404 y cabeceras falsas no cambian actor |
| E2E-CB-08 | Beneficio otorga período gratuito | Pendiente CB-05 |
| E2E-CB-09 | Cambio de membresía conserva historial | Aprobado local (09/11/2026): cambio a por clases con copia del tipo e historial preservado |
| E2E-CB-10 | Canje actualiza saldo sin sobregiro concurrente | Fuera de alcance: canjes descartados (D-76) |

## Dependencias y orden de trabajo

CB-01 mantiene la trazabilidad durante todas las etapas. CB-03 corrigió la
divergencia de seguridad en local; revisar evidencia y claves antes de habilitar.
CB-07 reutiliza sus roles y debe implementar la matriz fina y administración.
CB-04 y el diseño de CB-05 preceden al otorgamiento real de membresías promocionales.
CB-06 debe conservar las invariantes ya existentes. CB-08 cierra el conjunto,
sin sustituir la revisión de otro integrante por una autoaprobación del asistente.

CB-04 está implementada (09/11/2026). Lo que sigue del Club es CB-05 (recompensas,
con ADR previo) y CB-07 (administración); la prioridad general del proyecto la
define el equipo según el plan de etapas. No adelantar el diseño frontend sin decisión.

La asignación del proveedor externo de D9/etapa 5 sigue pendiente: ofrecer nuestra
API a Estudiario no prueba que consumimos una capacidad real del otro grupo.

## Sincronización de documentos Word

Registro_de_Decisiones_Gimnasio.docx refleja las tablas de DECISIONES.md, incluida
la propuesta D-70 separada de decisiones confirmadas. Los Word v2 de etapas y
prompts conservan su texto original y llevan aviso/adenda del 06/11/2026; las
restricciones antiguas no prevalecen sobre el nuevo alcance. Fuentes de sus adendas:
este plan y PROMPTS_CLUB_BENEFICIOS.md. No se renumeran sus etapas originales.

`scripts/sync-club-docs.ps1` sincroniza esos tres archivos, conserva copias previas
en la carpeta temporal y solo cambia word/document.xml del paquete. Ejecutar desde
la raíz con PowerShell (lectura UTF-8 explícita, sin cambiar política global):

```powershell
& ([scriptblock]::Create((Get-Content scripts/sync-club-docs.ps1 -Raw -Encoding UTF8)))
& ([scriptblock]::Create((Get-Content scripts/sync-club-docs.ps1 -Raw -Encoding UTF8))) -Check
```

Primero actualizar las fuentes Markdown. `-Check` comprueba coincidencia sin
escribir. La integridad XML/ZIP y sus estilos/partes conservadas se verificaron;
esto no sustituye una inspección visual en Word ni la revisión de otro integrante.

## Diseño frontend preparado para etapa futura

D-71 incorpora la revisión del brief y una propuesta de entrada en movimiento.
El plan FD-01–05 está en docs/PLAN_DISENO_FRONTEND.md; el brief completo y el
storyboard están en docs/diseno/. FD-01 tiene documentación preparada y revisión
pendiente; las demás etapas siguen pendientes. FD-04 se integra en las etapas
20A (alumno), 20B (profesional) y 20C (administrador) del Word original.
La propuesta D-72/ADR-019 conserva el stack y requiere validar prototipo, assets,
accesibilidad y rendimiento; habilitar identidad requiere revisar CB-03 y las
claves/configuración, aunque D-70 ya se haya corregido en local.
