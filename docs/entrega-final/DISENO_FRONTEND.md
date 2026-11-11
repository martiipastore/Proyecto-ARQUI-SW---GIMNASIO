# Dirección estética del frontend — Entrega Final

## 1. Alcance

Este documento adapta el brief `Brief_Diseno_Web_Metal_Fitness_Codex` al proyecto universitario. Su alcance es exclusivamente estético: identidad visual, composición, componentes, interacción, responsive y accesibilidad.

No define funcionalidades, reglas, permisos, datos ni contratos. Esos aspectos provienen de `SPEC.md`, `docs/ARCHITECTURE.md`, los ADR y OpenAPI. Ante cualquier contradicción, prevalecen esas fuentes.

## 2. Concepto visual

La interfaz debe sentirse como una evolución digital de la marca METALFITNESS: industrial, deportiva, enérgica, oscura, auténtica y cuidada. Debe evitar la apariencia de una plantilla genérica de gimnasio.

### Paleta inicial

| Token | Referencia | Uso |
|---|---:|---|
| `color-bg` | `#080A0A` | Fondo principal |
| `color-surface` | `#141719` | Navegación y superficies |
| `color-surface-raised` | `#24282A` | Tarjetas, tablas y diálogos |
| `color-text` | `#FFFFFF` | Texto principal |
| `color-text-muted` | `#BFC4C3` | Texto secundario |
| `color-accent` | `#A7DA00` | CTA, selección y datos destacados |

Los valores son aproximados hasta disponer del logotipo o manual de marca oficial. El verde lima se reserva para acciones y énfasis; no debe dominar toda la pantalla.

### Tipografía y formas

- Títulos: sans condensada y pesada, preferentemente Barlow Condensed o una alternativa aprobada.
- Texto y controles: Inter o Manrope.
- Títulos comerciales en mayúsculas; formularios, tablas y mensajes priorizan legibilidad.
- Bordes rectos o suavemente redondeados entre 8 y 14 px.
- Líneas diagonales, chevrons y cortes gráficos usados con moderación.
- Transiciones de interfaz (hover, foco, menús, toasts) entre 180 y 320 ms, sin movimientos que dificulten la lectura. Los momentos de la firma visual (§3.3) son la excepción deliberada.

## 3. Dos áreas, una identidad

### 3.1 Sitio público

El sitio institucional puede incluir:

- navegación y acceso a la aplicación;
- hero de la marca;
- presentación del gimnasio;
- actividades definidas en el dominio;
- tipos de membresía vigentes en `SPEC.md`;
- presentación informativa del Club de Beneficios;
- profesionales, contacto, ubicación y enlaces oficiales cuando sus datos estén confirmados.

Los CTA de contacto pueden dirigir a WhatsApp. El CTA principal de usuarios registrados debe permitir ingresar al sistema.

### 3.2 Aplicación autenticada

La aplicación conserva la paleta y la tipografía, pero utiliza una composición orientada a tareas:

- `AppShell` con navegación adaptada al rol;
- encabezado con identidad, contexto y cierre de sesión;
- tarjetas de resumen;
- tablas, formularios, filtros y paginación;
- estados de carga, vacío, error y acceso denegado;
- confirmaciones claras para operaciones sensibles;
- notificaciones breves y foco accesible después de cada acción.

### 3.3 Firma visual: momentos que tienen que impresionar

> Agregado el 10/11/2026 a pedido del equipo (D-89). Retoma y amplía la entrada
> en movimiento propuesta en D-72/ADR-019. Esta sección pasa a ser obligatoria
> para la etapa 11.

**Objetivo.** El sitio tiene que generar un «wow» en la primera impresión y en la
presentación, sin dejar de ser rápido y usable. Se logra con **pocos momentos muy bien
hechos**; el resto de la interfaz es sobria y prolija. Muchas animaciones repartidas
abaratan el resultado.

**Momento 1 — Entrada del sitio público: «la barra se carga».**

- Al abrir el inicio, discos de pesas entran desde los costados con inercia y se
  encastran en una barra. Al quedar alineados, la barra se tensa, el logotipo
  METALFITNESS se «forja» sobre ella y el acento lima se enciende como una chispa que
  recorre el borde.
- Duración total de 1,6 a 2,2 s, con aceleraciones de peso real (entrada rápida, frenada
  seca y un rebote mínimo al encastrar).
- El título, el texto y el botón **Ingresar** son visibles y utilizables desde el
  primer instante: la animación decora, no bloquea. Por decisión del equipo, se
  reproduce cada vez que se carga o actualiza el inicio.

**Momento 2 — Recompensas dentro de la aplicación.**

- **Reserva confirmada:** un disco se carga en una barra pequeña junto al mensaje de
  éxito (300 a 600 ms).
- **Puntos del Club:** el saldo sube animado con contador y un destello lima breve al
  cambiar. Las penalizaciones bajan sin destello.
- **Plan nuevo, medición o respuesta del nutricionista:** la tarjeta aparece con un
  corte diagonal.

Estos momentos se eligen porque son el recorrido principal de la demostración (D-85).

**Momento 3 — Ritmo del sitio público al desplazarse.**

- Las secciones entran con cortes diagonales y desplazamientos cortos coordinados con
  el scroll. Los chevrons de fondo tienen paralaje sutil.
- Las tarjetas de actividades reaccionan al cursor con inclinación leve y un borde lima.

**Reglas que no se negocian.**

1. **Tecnología:** SVG, CSS (keyframes, transiciones y animaciones ligadas al scroll
   cuando el navegador las soporte) y la API Web Animations. **Sin librerías nuevas**
   (D-49). Usar 3D (por ejemplo Three.js) o librerías de animación (GSAP, Framer Motion)
   requiere justificar por qué no alcanza lo anterior y consultarlo como excepción.
2. **Rendimiento:** animar solo `transform` y `opacity`. Sin saltos de diseño
   (CLS ≈ 0) y fluido a 60 fps en un equipo de gama media. La entrada pesa menos de
   40 KB (SVG y CSS) y no retrasa la primera pintura del contenido.
3. **Accesibilidad:** con `prefers-reduced-motion` se muestra directamente el estado
   final, estático y con la misma composición. Nada parpadea más de 3 veces por segundo.
   El movimiento nunca es la única forma de transmitir un estado.
4. **Marca:** la animación usa el logotipo oficial cuando esté disponible. Mientras
   tanto se usa una versión tipográfica provisoria, sin redibujar el logo (§8).
5. **Validación:** antes de integrar se muestra un prototipo funcionando y se revisa
   en 1440 y 375 px, con y sin movimiento reducido.

## 4. Mapa visual de pantallas

La lista describe presentación y navegación. El comportamiento se rige por `SPEC.md`.

### Públicas

- Inicio institucional.
- Login.
- Acceso de ingreso por DNI, presentado como modo de recepción o terminal.

### Alumno

- Resumen de membresía y próximas clases.
- Búsqueda de clases con filtros, orden y paginación.
- Detalle de clase, reserva y cancelación.
- Historial de reservas.
- Saldo y movimientos del Club de Beneficios. No hay catálogo ni canjes: el equipo los descartó (D-76).
- Plan de entrenamiento.
- Plan nutricional y mediciones.
- Consultas al nutricionista.
- Capacidad externa, cuando el proveedor y su función estén confirmados.

### Administrador

- Gestión de alumnos y profesionales.
- Asignaciones entre profesionales y alumnos.
- Tipos y asignaciones de membresía.
- Actividades, horarios y clases.
- Integraciones del Club (partners), vinculaciones y cuenta de puntos de cada persona con reversión de movimientos (etapa 7).
- Asistencias y movimientos de puntos.
- El estado de las notificaciones no tiene pantalla: se consulta en logs y en el tablero de observabilidad (D-83).
- Vista operativa del sistema cuando resulte útil para la demostración.

### Profesor

- Clases a cargo y alumnos visibles.
- Gestión de planes de entrenamiento de alumnos asignados.

### Nutricionista

- Pacientes asignados.
- Planes alimenticios y mediciones.
- Consultas recibidas y respuestas.

## 5. Componentes reutilizables

### Navegación y estructura

- `PublicHeader`, `AppSidebar`, `AppHeader`, `MobileNavigation` y `PageHeader`.
- Breadcrumbs solo en niveles donde reduzcan ambigüedad.
- Menú móvil accesible, con foco contenido y cierre por teclado.

### Datos y acciones

- `DataTable`, `FilterBar`, `Pagination`, `SearchInput` y `StatusBadge`.
- `FormField`, `DateField`, `SelectField`, `ErrorSummary` y validación junto al campo.
- `PrimaryButton`, `SecondaryButton`, `DangerButton` e `IconButton` con nombres accesibles.
- `ConfirmDialog`, `Toast`, `EmptyState`, `ErrorState` y `Skeleton`.
- Tarjetas específicas para clases, membresías, planes y movimientos de puntos sobre una base visual común.
- Componentes de la firma visual (§3.3): `HeroBarbell` (entrada), `PlateLoad` (confirmación) y `AnimatedCounter` (puntos), cada uno con su versión estática.

## 6. Criterios visuales por flujo

- La disponibilidad y ocupación de una clase deben leerse sin depender solamente del color.
- El estado de membresías, reservas, movimientos y notificaciones debe combinar texto, forma e iconografía.
- Los errores del backend deben presentarse en lenguaje comprensible, conservando un identificador de correlación cuando esté disponible.
- Las escrituras deben bloquear dobles envíos visuales y mostrar progreso; la garantía real depende de las reglas de idempotencia del backend.
- Los filtros activos deben ser visibles y fáciles de limpiar.
- Las vistas del Club de Beneficios usan el mismo lenguaje oscuro; no se copia la estética blanca mencionada en el brief.

## 7. Responsive y accesibilidad

- Verificar como mínimo 1440, 1024, 768 y 375 px, sin desplazamiento horizontal.
- Usar tipografía fluida con `clamp()` donde corresponda.
- Mantener objetivos táctiles cercanos a 44 px.
- Permitir navegación completa por teclado y foco visible.
- Mantener contraste suficiente sobre fondos y fotografías.
- Respetar `prefers-reduced-motion`.
- Utilizar HTML semántico, etiquetas explícitas y anuncios accesibles para resultados asincrónicos.
- En móvil, transformar tablas extensas en tarjetas o vistas resumidas sin ocultar información necesaria.

## 8. Rendimiento y recursos

- Fotografías oficiales en WebP o AVIF, con dimensiones declaradas y `srcset`/`sizes`.
- Carga diferida fuera del contenido inicial.
- No descargar material de Instagram ni utilizar fotografías del local sin autorización.
- No redibujar ni reemplazar el logotipo por uno inventado.
- Mientras falten recursos, usar placeholders explícitos y mantener un inventario de pendientes.

## 9. Datos que no se tomarán del brief

No se incorporarán sin validación en las fuentes funcionales:

- precios o modalidades de membresía distintas de `SPEC.md`;
- días, horarios, cupos o profesores inventados;
- descuentos, promociones o códigos observados en capturas;
- testimonios, métricas comerciales o características no verificadas;
- permisos, roles o funcionalidades nuevos;
- restricciones sobre implementar autenticación, administración o backend.

## 10. Recursos pendientes

- Logotipo oficial en formato adecuado para web.
- Fotografías autorizadas del gimnasio, actividades y profesionales.
- Confirmación de dirección, horarios de atención y canales oficiales.
- Copys institucionales aprobados.
- Definición visual de la capacidad externa cuando se conozca su función.

## 11. Criterios de aceptación estética

- Sitio público y aplicación autenticada comparten tokens y se reconocen como un único producto.
- La interfaz conserva predominio oscuro y acentos lima estratégicos.
- Todas las pantallas necesarias tienen estados de carga, vacío, error y éxito.
- Desktop y móvil mantienen jerarquía, legibilidad y acceso a las mismas operaciones.
- Ninguna decisión estética modifica una regla del dominio ni oculta información necesaria para operar.
- Los tres momentos de la firma visual (§3.3) están implementados, cumplen sus reglas de rendimiento y accesibilidad, y fueron aprobados por el equipo sobre un prototipo funcionando.
- El build, chequeo de tipos y revisión visual se ejecutan antes de integrar cada recorrido.

