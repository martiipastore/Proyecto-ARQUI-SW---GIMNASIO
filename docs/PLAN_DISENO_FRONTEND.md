# Plan de diseño futuro — Metal Fitness

Fecha: 06/11/2026. D-71 confirma el pedido; D-72 y ADR-019 son propuestas para
revisión. No renumera etapas académicas, CB ni las del Word original.

| Etapa | Entregable y condición de cierre | Estado |
|---|---|---|
| FD-01 | Brief corregido con teoría, storyboard y pendientes comerciales identificados | Documentación preparada; revisión del equipo pendiente |
| FD-02 | Wireframes móvil/escritorio, navegación pública y áreas por rol; logo, fotografías, horarios, planes y promociones validados | Pendiente |
| FD-03 | Prototipo de movimiento de entrada; alternativa estática; comprobar teclado, preferencias, fallos y costo de carga antes de integrarlo | Pendiente |
| FD-04 | Implementación en aplicación existente: etapa 20A alumno, 20B profesional, 20C administrador; integrar contratos disponibles y conservar flujos | Pendiente |
| FD-05 | Validación funcional, visual, accesibilidad y rendimiento con evidencia; revisión previa a publicación | Pendiente |

FD-02 puede avanzar en diseño mientras continúa backend. FD-04 depende de los
contratos y de CB-03 para identidad/roles, CB-04/05 para membresías/recompensas,
CB-06 para canjes y CB-07 para administración. No mostrar operaciones pendientes
como funcionalidades ya operativas. Resolver D-70 antes de habilitar acceso.

La propuesta principal de entrada dura aproximadamente 1,8 s: discos se alinean
en una barra, con texto y CTA utilizables desde el inicio, silencio y versión
estática. Estos parámetros no están aprobados como diseño final ni medidos.

> **Actualización del 10/11/2026 (D-89):** la entrada en movimiento pasa a ser
> obligatoria y se amplía a tres momentos de firma visual en
> [DISENO_FRONTEND.md §3.3](entrega-final/DISENO_FRONTEND.md), con duración de 1,6 a
> 2,2 s y reglas de rendimiento y accesibilidad. FD-03 se ejecuta como prototipo de la
> etapa 11 de la entrega final. Las referencias de este plan a canjes (CB-06) quedaron
> sin efecto por D-76.

Fuentes: [brief completo](diseno/BRIEF_WEB_METAL_FITNESS_V2.md),
[storyboard conceptual](diseno/storyboard-entrada-metal-fitness.svg),
[ADR-019](adr/ADR-019-experiencia-web-movimiento-progresivo.md).
