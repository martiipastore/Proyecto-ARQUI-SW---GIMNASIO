# Etapa 11 — Frontend completo y coherencia visual

**Estado:** validación funcional local cerrada el 11/11/2026. Se recorrieron desde
la interfaz, con el stack completo en Docker, todas las pantallas de los cuatro
roles y el recorrido principal de punta a punta. `tsc` y `vite build` aprobados.
Falta la revisión de otro integrante. No se publicó en Vercel: producción no tiene
Booking ni Training (D-75).

Fuentes: [DISENO_FRONTEND.md](DISENO_FRONTEND.md) (estética, D-84 y D-89) y los
contratos de [docs/api](../api/README.md). El diseño no cambió reglas, permisos ni
contratos.

## 1. Qué se hizo

**Identidad visual.** Paleta y tipografías tal como están en el código (lima
`#EAFF21`, Impact/Segoe UI), confirmadas por el equipo el 11/11/2026 en lugar de los
valores aproximados de DISENO §2. Marca tipográfica provisoria (sin logo oficial).

| Área | Pantallas |
|---|---|
| Pública | Inicio con la entrada «la barra se carga», login, activación de cuenta, estado del sistema, 403 y 404 |
| Estructura | `AppShell` con barra lateral por rol, barra superior y menú móvil |
| Alumno | Inicio (membresía, próximas clases, puntos), clases en tarjetas con ocupación, mis reservas, mi plan (entrenamiento, alimentación, consultas, mediciones), mi cuenta |
| Administrador | Personas y ficha (datos, baja, membresías, asignaciones, Club), actividades y horarios, clases y detalle, recepción por DNI, integraciones del Club |
| Profesor | Mis clases y detalle con alumnos, mis alumnos y ficha con planes |
| Nutricionista | Mis pacientes y ficha (plan alimenticio, mediciones), bandeja de consultas |

**Firma visual (DISENO §3.3).** Solo CSS y React, sin librerías nuevas (D-49);
animan `transform` y `opacity`.

| Momento | Implementación |
|---|---|
| 1. Entrada del sitio | `HeroBarbell`: discos que encastran, barra que se tensa, marca que se forja y chispa lima (~1,9 s). Se repite en cada carga del inicio (decisión del equipo) |
| 2. Recompensas | `PlateLoad` al confirmar una reserva; `AnimatedCounter` en el saldo, con destello solo al subir; corte diagonal (`cut-in`) en plan nuevo, medición y respuesta |
| 3. Ritmo al desplazarse | Secciones que entran con desplazamiento diagonal, paralaje de chevrons con `animation-timeline` y tarjetas de actividades que se inclinan hacia el cursor |

Con `prefers-reduced-motion` se muestra el estado final sin movimiento.

**Componentes comunes:** `PageHeading`, `SectionHeading`, `Notice` (aviso de éxito,
error o información con `role` accesible), `EmptyState`, `useConfirm` (diálogo
`<dialog>` que reemplaza `window.confirm` y `window.prompt`), `PlateLoad`,
`AnimatedCounter`. Formato de fecha y hora único en 24 h (`src/format.ts`).

**Accesibilidad y móvil:**
- El estado de clases, reservas y movimientos se comunica con texto e ícono, además
  del color.
- El diálogo de confirmación contiene el foco y se cierra con Escape.
- El menú móvil se cierra con Escape y al navegar.
- En pantallas de 620 px o menos, las tablas pasan a tarjetas.
- Los filtros activos de la búsqueda de clases se ven y se pueden limpiar.

## 2. Validación desde la interfaz (11/11/2026)

Stack completo en Docker; operaciones hechas con el navegador, sin llamadas manuales.

| Rol | Operaciones verificadas |
|---|---|
| Administrador | Alta de alumno, profesor y nutricionista con mail de activación en Mailpit; alta duplicada rechazada; editar datos; asignar, renovar y cancelar membresía; asignar y revocar profesional; horarios (con rechazo por profesor superpuesto) y generación de clases; inactivar y activar actividad; cambiar capacidad y cancelar clase con diálogo; recepción por DNI (verde, «bienvenido de nuevo», DNI inexistente, sin reserva y persona dada de baja); ver cuenta de puntos, revertir movimiento y vincular cuenta externa; crear integración (key visible una vez), rotar key y desactivar; dar de baja |
| Activación | Contraseñas que no coinciden, activación correcta, login con contraseña incorrecta |
| Alumno | Inicio sin membresía y con membresía; reserva con `PlateLoad`; rechazo por membresía no vigente; cancelación con diálogo; filtros y limpieza; saldo de 500 puntos con su movimiento; reversión visible; consulta al nutricionista y respuesta; plan, alimentación y medición |
| Profesor | Mis clases con ocupación; detalle con el alumno y su ingreso; crear plan y reemplazarlo (el anterior queda en el historial); acceso a alumno no asignado rechazado; ruta de otro rol con «Acceso denegado» |
| Nutricionista | Mis pacientes; plan alimenticio; medición; responder consulta |
| Móvil (375 px) | Sin desplazamiento horizontal en las 18 pantallas autenticadas de los cuatro roles |

Recorrido principal (ETAPA_0 §4, D-85): alta → membresía → mail → login → búsqueda →
reserva → ingreso por DNI → `asistencia.registrada` → Benefits acredita 500 puntos una
vez → el alumno ve saldo y movimientos.

## 3. Hallazgos y correcciones

| Hallazgo | Tratamiento |
|---|---|
| El alumno veía su saldo como «Pendiente» aunque Benefits ya había acreditado los puntos | `GET /benefits/me` y `/benefits/me/movements` solo se registran con `BENEFITS_EXTERNAL_REGISTRATION_ENABLED=true` (el E2E de la etapa 6 lo prende). Se activó en el `.env` local y se aclaró en `.env.example`. Requiere `GYM_INTERNAL_SERVICE_KEY` de 32 caracteres o más, compartida con el gateway |
| Horas en 12 h sin a. m./p. m. («01:05» en lugar de «13:05») | Formato único de 24 h con zona Argentina |
| Botones destructivos en lima | Rojo para baja, cancelación y reversión; gris para revocar y volver |

## 4. Pendiente

- Revisión de otro integrante y validación visual del equipo sobre el prototipo en
  1440 y 375 px, con y sin movimiento reducido (DISENO §3.3, regla 5).
- Recursos oficiales: logotipo, fotografías, textos institucionales y datos de
  contacto (DISENO §10). Los textos del sitio público son provisorios.
- Pantalla de la capacidad externa (etapa 13, MELT).
- La base local conserva integraciones de pruebas anteriores («Demo etapa 3»,
  «Fixture resiliencia»); desactivarlas antes de una demostración si molestan.
