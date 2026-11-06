# Prompts de continuación del Club — 06/11/2026

Adenda vigente para el Club de los prompts v2 del 08/10/2026. Conservar ese
documento como historia; no reutilizar su D-06/D-08/D-19/D-28 para revertir el
alcance aprobado. Las etiquetas CB no renumeran las etapas del Word v2 ni las
ocho etapas académicas de PLAN_ENTREGA_2.md.

## Instrucción común para todas las etapas CB

Trabajá sobre el repositorio existente. Antes de editar leé CLAUDE.md, SPEC.md y
su adenda del Club, DECISIONES.md, PLAN_CLUB_BENEFICIOS.md y los ADR afectados.
Reutilizá el código válido, preservá datos y modificaciones previas. Separá requisito,
decisión propuesta, implementación local, prueba ejecutada y despliegue. Documentá
alternativas y justificación académica sin atribuir algoritmos o cifras al PDF.
No supongas aprobación técnica por estar escrito o implementado. Mantené registro
Markdown/Word, etapas y ADR sincronizados; no hagas commits, push ni cambios de
producción o permisos externos por iniciativa propia.

## CB-01 — Conciliar alcance y documentación

Comprobá D-63 a D-70 y HU-CB-01–08/RN-CB-01–15 contra el pedido del equipo y el
código. Conservá decisiones reemplazadas. Marcá revisión de otro integrante y
evidencias faltantes. Validá enlaces, unicidad de IDs y sincronización de los Word;
documentar no equivale a aprobar un PR ni a terminar el Club.

## CB-02 — Verificar integraciones y puntos

Revisá migraciones 003/004/005, permisos, credenciales, revocación, cuotas y límites
configurables. Validá evento único independiente de key, retención del evento,
concurrencia de presupuesto entre cuentas y rollback. Corrigí sin debilitar saldo
no negativo, secuencia o inmutabilidad. Ejecutá PostgreSQL real y compatibilidad
sin eventId; si se omiten, registralo y no cierres la etapa.

Estado al 06/11/2026: CB-02 implementada y validada localmente, revisión del equipo
pendiente. Consultar CB02_VALIDACION.md y evidence/club/cb02-tests.json antes de
repetir trabajo. Nuevas integraciones exigen evento por defecto; clientes previos
conservan compatibilidad explícita. CB-03/D-70 se validó posteriormente; el trabajo
técnico actual continúa en CB-04, según el plan vigente.

## CB-03 — Conciliar seguridad y cerrar registro/acceso

Estado al 06/11/2026: D-70 corregida localmente según ADR-006: JWT RS256 900 s,
roles persistidos y bcrypt mínimo 12. Consultar CB03_VALIDACION.md, informes de
tests/E2E y ADR-017 propuesto. No reintroducir sesiones opacas ni editar ADR-006.
Logout v1 descarta en cliente sin revocar copias; no hay refresh. Revisar claves,
evidencia y aprobación del equipo antes de habilitar. El siguiente trabajo es CB-04.
Después verificá alta/reutilización sin duplicados, prueba de control de correo,
origen, token de un solo uso, contraseña inicial única, outbox, recuperación tras
commit remoto, fallo SMTP, acceso propio, revocación y cabeceras falsas por gateway.
No habilites flags ni producción mientras esa conciliación esté pendiente.

## CB-04 — Implementar membresías sin perder puntos

Implementá en Members tipos mensual, trimestral, anual y por clases, estados,
vigencia y renovación con historial. No borres ni reinicies cuentas/movimientos de
Benefits al vencer o cambiar membresía. Cantidades por clases, precios y condiciones
se configuran, sin inventar reglas definitivas. Probá vencimiento, retorno y cambio
de tipo con saldo/historial idénticos salvo operaciones de puntos válidas.

## CB-05 — Reglas y recompensas especiales

Prepará el ADR correspondiente antes de implementar. Configurá acciones por
integración, elegibilidad por membresía/estado, fechas y restricciones. Diferenciá
puntos, recompensa, descuento y período promocional; confirmación autorizada y
evento único para cada efecto. Probá período gratuito sin doble otorgamiento ni
equivalencia arbitraria en puntos. Los ejemplos comerciales no fijan porcentajes.

## CB-06 — Canjes y ajustes

Extendé dominio, transacciones y restricciones del ledger juntos. Implementá
canjes y ajustes autorizados, justificados e idempotentes; preservá historial.
Probá solicitudes concurrentes sin sobregiro y saldo actualizado en consulta
propia y externa. No introduzcas débitos fuera del saldo conciliado.

## CB-07 — Panel y administración real

Usá roles y autorización de CB-03 antes de exponer operaciones administrativas.
Agregá búsqueda de usuarios, saldo/historial, origen y membresía, acreditación
justificada, promociones y gestión de integraciones/permisos/límites/reglas.
Claves visibles una sola vez al emitir/rotar y persistidas como hash; auditoría de
actor, motivo y cambios, sin secretos en frontend ni logs. Incluí diagnóstico y
recuperación controlada de comandos pendientes.

## CB-08 — Validación integral y despliegue

Ejecutá E2E-CB-01–10, Spectral y pruebas reales de base/correo. Repetí resiliencia
y medí el nuevo flujo sin reciclar resultados de etapas anteriores. Verificá
migraciones, secretos y limpieza técnica sin purgar ledger/eventos. Inspeccioná
GitHub/Vercel solo con acceso real y documentá conexión, ramas, colaboradores y
variables. Indicá cambios necesarios antes de tocar producción o permisos ajenos.
No declares D9 resuelta por ofrecer nuestra API: falta consumir un proveedor real.

## Consigna para la futura etapa de frontend

Antes de implementar, leer docs/PLAN_DISENO_FRONTEND.md, el brief v2 en
 docs/diseno/BRIEF_WEB_METAL_FITNESS_V2.md y ADR-019 (propuesto). Conservar la
aplicación y los flujos privados existentes. Validar assets comerciales y
wireframes; prototipar la entrada de discos que se alinean en una barra con CTA
inmediato, silencio y alternativa estática. La duración sugerida de 1,8 s no es
una regla aprobada. No agregar bibliotecas o capas sin necesidad justificada.
Aplicar estados procesando/confirmado/rechazado/verificando y reintentos con la
misma intención según contrato. No inventar saldos, promociones ni membresías.
Implementar en 20A/B/C cuando corresponda y registrar pruebas FD-05. D-70 y los
contratos reales son dependencias; redactar el brief no completa frontend.
