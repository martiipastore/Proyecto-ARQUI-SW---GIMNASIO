# METAL FITNESS — Brief web revisado y concepto de movimiento

Versión 2 preparada el 6 de noviembre de 2026 para revisión del equipo.
Especificación para la futura etapa de frontend; no es una orden de implementarla
ahora ni acredita que sus pantallas o animaciones ya existan.

Fuente visual: Brief_Diseno_Web_Metal_Fitness_Codex (1).docx facilitado por el
usuario. Fuente arquitectónica: Teórico EN PDF.pdf, 149 páginas, revisado directamente.
Alcance funcional: SPEC 0.11, su adenda del Club y decisiones D-63 a D-70.
La solicitud de movimiento está confirmada; el concepto concreto, sus tiempos y
presupuestos siguientes son propuestas de diseño, no reglas fijadas por el teórico.

## 1. Objetivo y dirección

Crear una experiencia reconocible de Metal Fitness: industrial, oscura, energética,
con acentos lima, fotografía auténtica y una entrada con movimiento propia de la
marca. La home debe invitar a conocer el gimnasio y contactar; el área de usuarios
debe permitir operar con claridad y seguridad, incluso cuando una operación tarda
o una dependencia falla. No rehacer el proyecto ni sacrificar sus funciones actuales.

Se conserva del brief original: negro/grafito/lima, títulos condensados, fotografías
reales, diagonales moderadas, tarjetas, componentes reutilizables y contenido
verificable. Se amplía con experiencia privada, estados de negocio, fuentes de
datos, criterios comprobables y una especificación concreta de motion.

## 2. Límites y disponibilidad de referencias

El Word original menciona capturas de Instagram, logo y referencias de comercios,
pero no contiene imágenes incrustadas. No se afirma haber contrastado el feed, el
logo original ni las capturas ausentes. Los HEX, familias tipográficas, copys y
composiciones siguen sujetos a validación visual con los assets oficiales.

El teórico es de arquitectura y sistemas distribuidos: fundamenta límites, APIs,
recuperación y experiencia ante fallos; no prescribe colores, una estética fitness,
accesibilidad visual ni animaciones. Estas últimas decisiones se apoyan en criterios
de diseño y fuentes complementarias declaradas al final.

Este brief no incorpora cobros, checkout, reservas por WhatsApp ni porcentajes
comerciales nuevos. Mostrar información de un plan no equivale a venderlo ni a
activar una membresía. Las instrucciones antiguas de excluir registro o panel no
deben usarse para eliminar lo requerido por el Club: su integración se hará cuando
las capacidades y permisos de backend estén disponibles.

## 3. Correcciones respecto del brief original

| Tema | Criterio revisado |
|---|---|
| Stack alternativo si el proyecto está vacío | Ya existe React, TypeScript, Vite y CSS. Reutilizarlos; no migrar a Tailwind, otro framework o un proyecto paralelo por este brief. |
| Una home como experiencia completa | Distinguir web pública, ingreso/activación y áreas privadas por rol. Conservar el acceso operativo existente. |
| Planes de capturas: 8/12 clases, semestral, pase libre | No son tipos o precios aprobados. El dominio contempla mensual, trimestral, anual y por clases; cantidades, etiquetas comerciales y tarifas necesitan confirmación. No agregar semestral por una captura. |
| Horarios de Instagram frente al SPEC | Apertura comercial y turnos reservables son datos distintos. No convertir el horario observado 6:00–00:00 en disponibilidad para reservar. El SPEC conserva sus turnos aprobados; validar el horario público antes de mostrarlo. |
| Beneficios en un archivo estático | El contenido editorial puede ser configurable. Saldo, movimientos, elegibilidad, vigencia y canjes provienen de las APIs del gimnasio y sus reglas; no se resuelven copiando porcentajes a un JSON público. |
| Beneficios exclusivos para socios activos | Una cuenta externa puede tener puntos sin membresía. Mostrar requisitos de cada beneficio y conservar acceso al saldo aunque no haya membresía activa. |
| Animación de entrada genérica | Definir una escena breve, controlada y accesible; nunca una pantalla de espera obligatoria. |
| Diseño primero desktop | Trabajar desde móvil y comprobar también desktop; formularios y lecturas privadas merecen tanta atención como el hero. |

## 4. Sistema visual propuesto

| Token / recurso | Dirección |
|---|---|
| Fondo principal | Negro casi puro #080A0A |
| Superficie de tarjetas | Carbón #141719; nivel elevado #24282A |
| Texto principal / secundario | Blanco #FFFFFF / gris #BFC4C3 |
| Acento | Lima aproximado #A7DA00; validar con logo oficial |
| CTA primario | Fondo lima y texto negro; no blanco sobre lima |
| Títulos de marketing | Sans condensada y pesada, una familia por confirmar; mayúsculas solo en titulares breves |
| Texto, formularios y tablas | Sans legible; evitar condensada y mayúsculas sostenidas en instrucciones o condiciones |
| Composición | Márgenes amplios, diagonales localizadas, bordes simples y radios coherentes; no saturar con franjas verdes |
| Fotografía | Gimnasio y personas reales, autorizadas; recortes específicos para móvil y escritorio |

Usar tokens compartidos para color, tipografía, espacio, bordes, foco y motion.
Las proporciones de color originales son una orientación de arte, no porcentajes
rígidos que prevalezcan sobre legibilidad. Reservar lima para acciones y énfasis;
advertencias y errores también deben tener texto/icono, sin depender del color.

No deformar ni animar el logo oficial. Mientras falte, el wireframe puede indicar
«Logo pendiente» o usar el nombre tipográfico como marcador claramente provisional.
Seleccionar fuentes con licencia adecuada y fallback; no importar varias familias
de una red externa por defecto.

## 5. Audiencias y navegación

### Visitante

Navegación pública: Inicio, El gimnasio, Actividades, Instalaciones, Planes,
Beneficios y Contacto. Una acción comercial clara y un acceso secundario visible
«Ingresar / Mi cuenta». Evitar que ambos compitan con igual peso.

### Usuario con cuenta

Acceso a su información: puntos, movimientos y recompensas; membresía y reservas
cuando estén implementadas; entrenamiento/nutrición según las capacidades del SPEC.
«Sin membresía activa» informa su situación, no desactiva toda la cuenta ni oculta
los puntos. Distinguir falta de requisito de saldo insuficiente y de error de carga.

### Profesional y administrador

Layouts de trabajo propios, con navegación y densidad adecuadas. El profesional
solo ve sus recursos autorizados. El administrador necesita usuarios, membresías,
beneficios, promociones, puntos e integraciones. Ocultar un botón no autoriza una
operación: el backend verifica roles y acceso. No mostrar credenciales existentes
ni pedir al usuario API keys técnicas en pantallas del Club.

Son áreas de una misma aplicación, no tres proyectos ni tres BFF nuevos.
La animación de portada no se repite en cada ingreso, tabla, formulario o canje.

## 6. Home pública y jerarquía de contenido

| Bloque | Contenido y acción |
|---|---|
| Header | Logo, navegación accesible, contacto configurable e Ingresar. Sticky sin tapar títulos, foco o anclas. |
| Hero | Titular propuesto «ACTIVÁ TU MEJOR VERSIÓN», una frase breve, CTA «Conocé los planes» y secundario «Ver actividades». Texto y botones utilizables desde el primer instante. Movimiento decorativo en zona propia. |
| El gimnasio / instalaciones | Fotos reales y relato breve; sin estadísticas, testimonios o equipamiento inventados. Galería sin carrusel automático obligatorio. |
| Actividades | Musculación, Funcional, GAP, Strong Nation y Zumba. Tarjetas con descripción, imagen y consulta de agenda real o contacto si no está disponible. |
| Planes | Tipos aprobados y condiciones verificadas. Si faltan precios, mostrar consulta, no $0, cuotas ficticias ni ofertas vencidas. CTA comercial no confirma pago o membresía. |
| Club de Beneficios | Explicar puntos, recompensas y condiciones; catálogo público publicable y acceso al área personal. No prometer que todo beneficio aplica a toda membresía. |
| Acompañamiento | Equipo real con nombre, rol, foto y permiso confirmados. Sin profesionales de stock presentados como integrantes. |
| Contacto / footer | Dirección, horarios y canales confirmados. Mapa o enlace a Maps, Instagram y WhatsApp por acción del visitante. Evitar widgets sociales pesados al abrir la página. |

Contacto de WhatsApp y mensajes prearmados son configurables, sin incluir datos
sensibles o tokens. Si falta un dato, el prototipo lo marca como pendiente; la
versión pública no publica ese marcador como hecho ni enlaza rutas inexistentes.

## 7. Club de Beneficios y experiencia privada

Pantalla de cuenta: saldo con fecha de actualización, movimientos, recompensas
disponibles/otorgadas y un resumen separado de membresía. Catálogo con búsqueda y
filtros útiles por categoría y condiciones. No mezclar saldo de puntos, descuento
por convenio y un mes promocional como si fueran una misma moneda.

Cada beneficio muestra nombre, proveedor, tipo de recompensa, vigencia, requisitos,
condiciones completas y acción válida. El backend determina elegibilidad y saldo.
Un beneficio no elegible explica qué falta; uno vencido no se presenta como vigente.
Las categorías y porcentajes de las capturas son ejemplos pendientes, no contenido
certificado de Botánico, Pupi, Marea, Lo Más Rico o Kinesio Training.

Una persona que llega desde una plataforma externa debe entender que ya tiene una
cuenta, cómo activar su acceso y que no necesita contratar una membresía para
conservar puntos. Confirmar vínculo de una cuenta previa no restablece su contraseña.

### Estados que deben diseñarse

| Estado | Comunicación y comportamiento |
|---|---|
| Cargando | Indicador acotado y estructura estable; no inventar saldo o disponibilidad. |
| Vacío | «Todavía no tenés movimientos» u otro mensaje específico, con acción pertinente. |
| Procesando | La intención fue recibida; conservar su referencia y evitar una segunda intención accidental. |
| Confirmado | Mostrar éxito solo con confirmación durable del backend; actualizar saldo y detalle. |
| Rechazado | Explicar el requisito incumplido o dato corregible, conservando lo que el usuario ingresó. |
| Verificando / resultado desconocido | Un timeout no significa rechazo. Consultar el estado de la misma operación; no generar otra key para repetir el efecto. |
| Datos anteriores | Identificar fecha/contexto y ofrecer actualización; un saldo anterior no autoriza un canje. |
| Sesión vencida / sin permiso | Explicar cómo volver a ingresar o qué acceso falta, sin exponer información ajena. |

No mostrar «0 puntos», «sin clases» o «membresía inactiva» como sustitutos de un
error de conexión. Usar mensajes orientados a la acción del usuario, no nombres
de microservicios, SQL o errores internos. Diseño apoyado en el teórico, p. 110.

## 8. Concepto principal de entrada: Fuerza en movimiento

Propuesta recomendada para prototipar: una barra y dos discos estilizados en carbón,
con un canto lima. Los discos descienden en diagonal y se alinean sobre la barra;
al asentarse, una línea lima recorre la base de la ilustración y queda estable.
Sugiere peso y energía con una sola resolución visual, sin lluvia interminable de
pesas, temblor de pantalla, destellos ni apariencia de accidente.

La escena ocupa un contenedor recortado a la derecha del hero en escritorio y una
franja propia debajo del copy en móvil. No cruza texto, navegación ni botones.
No presenta equipamiento ilustrado como fotografía del gimnasio ni altera el logo.
Ver storyboard separado: storyboard-entrada-metal-fitness.svg.

| Momento propuesto | Escena | Interacción |
|---|---|---|
| 0 ms | Copy, CTA y composición estática inicial ya visibles; espacio reservado para el gráfico. | Se puede navegar y usar botones inmediatamente. |
| 0–450 ms | Dos discos entran dentro de su zona, con desplazamiento diagonal breve y giro discreto. | No captura puntero, teclado ni scroll. |
| 450–1200 ms | Se alinean con la barra y desaceleran. | El contenido no cambia de tamaño ni se desplaza. |
| 1200–1800 ms | Asentamiento mínimo y trazo lima localizado; termina en una imagen estable. | No reinicia automáticamente. |
| Después | Hero quieto; opción discreta para repetir o desactivar movimiento. | Preferencia del visitante respetada. |

Duración inicial 1,8 segundos, ajustable tras prototipo; no es un valor del teórico.
Se propone una ejecución al entrar a la home durante la visita, sin volver a
dispararla al salir de un formulario o volver del área de cuenta. No usar espera
obligatoria, loader de marca, cuenta regresiva o botón «Saltar» para liberar el sitio.

La reproducción solo se intenta con el hero visible y sin preferencia de movimiento
reducido. Parar al ocultar la pestaña o salir de la zona. La decoración no entra en
el orden de foco ni duplica texto para lectores de pantalla; sus controles sí son
botones accesibles. Nunca reproducir sonido o vibración automáticamente.

## 9. Alternativa de movimiento con una persona entrenando

Si el gimnasio entrega un clip propio y autorizado, evaluar una toma corta de una
repetición controlada, con recorte móvil y poster real. Mantener copy y CTA como
HTML legible, no textos incrustados en video. Silencioso, sin bucle obligatorio y
con reproducción/pausa accesibles. Si autoplay se rechaza, conservar el poster y
permitir reproducción voluntaria.

No utilizar al mismo tiempo el clip de fondo y los discos animados en el primer
hero: elegir una dirección tras el prototipo. La propuesta de discos permite
explorar la identidad sin depender de una filmación aún inexistente. No generar
un supuesto instructor o local real para suplir esa ausencia.

## 10. Matriz de movimiento y degradación

| Condición | Experiencia esperada |
|---|---|
| Movimiento permitido, hero visible | Secuencia breve y única; control para desactivar o repetir. |
| prefers-reduced-motion: reduce | Composición final estática desde el inicio; sin desplazamiento, zoom o rebote. Cambio de preferencia durante la visita también detiene la escena. |
| Movimiento desactivado manualmente | Mantener la preferencia durante la visita; no reactivarlo por scroll o navegación. |
| Ahorro de datos indicado o conexión limitada detectada | Priorizar SVG/imagen estática; no descargar video decorativo automáticamente. No inferir condiciones que el navegador no informa. |
| Sin soporte de animación o fallo del recurso | Poster/ilustración estática; navegación y contenido continúan. |
| Autoplay de video rechazado | Poster y control de reproducción voluntaria, sin prometer inicio automático. |
| Fuera de pantalla / pestaña oculta | Detener el trabajo decorativo; no mantener procesamiento de una escena invisible. |

Si en el futuro se propone movimiento automático continuo de más de cinco segundos,
debe poder pausarse, detenerse u ocultarse según WCAG 2.2.2. Respetar movimiento
reducido no sustituye por sí solo ese control. Para este concepto se evita el bucle.

## 11. Accesibilidad y responsive

Meta de diseño: WCAG 2.2 AA, pendiente de auditoría; no declarar conformidad por
elegir colores oscuros. Probar contraste real, incluidos overlays y estados.
Texto normal al menos 4,5:1 y texto grande 3:1, según WCAG 1.4.3. En CTA lima usar
texto oscuro: blanco sobre ese lima no ofrece contraste suficiente para el texto.

Diseño desde 320/375 px y comprobaciones en 768, 1024 y 1440 px, además de tamaños
intermedios, orientación y zoom. Esos tamaños son referencias, no únicos dispositivos.
Evitar scroll horizontal de página; tablas amplias pueden necesitar un contenedor
accesible propio o una vista de tarjetas, sin esconder información relevante.

Proponer objetivos táctiles de 44 px como política del proyecto, no como el mínimo
AA universal. Foco visible, navegación por teclado, enlace para saltar al contenido,
menú móvil con gestión de foco, labels asociados, errores junto a campos y estados
anunciados sin lectura repetitiva. No depender de hover ni de gestos finos.

Titulares fluidos con clamp; formularios, condiciones e historial con tamaño cómodo
y lectura natural. La home puede ser expresiva; cuenta, canjes y administración
deben priorizar legibilidad, claridad y densidad adecuada.

## 12. Rendimiento y alternativas técnicas

Mantener la SPA y CSS del proyecto. Primera opción para la escena: SVG/CSS con
transform y opacity, sin biblioteca de física, WebGL o framework de animación nuevo.
Revisar la alternativa si el prototipo demuestra una necesidad real, mediante ADR.
Un BFF nuevo no se justifica solo por tener áreas públicas y privadas.

Contenido esencial visible sin esperar imágenes, fuentes o animación. Reservar
dimensiones/aspect ratio; imagen principal sin lazy loading; diferir galería y video
no prioritarios. El video decorativo no debe bloquear la primera vista y necesita
poster, fuentes compatibles y estrategia de descarga según preferencias.

Presupuestos iniciales a medir, no resultados obtenidos ni cifras del teórico:
SVG/motion comprimido hasta 80 KB; imagen de hero móvil alrededor de 200 KB como
objetivo; ningún paquete nuevo exclusivamente para el efecto inicial. Si un asset
oficial no alcanza el presupuesto sin perder calidad, revisar composición y medir.
No asumir que esos límites bastan para garantizar fluidez.

Objetivos de experiencia a validar: LCP ≤2,5 s, INP ≤200 ms y CLS ≤0,1, referencias
de Core Web Vitals al percentil 75 cuando haya datos reales. Antes del despliegue,
documentar pruebas de laboratorio con dispositivo/red declarados; no presentarlas
como datos de campo ni como métricas backend de las etapas anteriores.

## 13. Fuentes de contenido y reglas de edición

| Información | Fuente y criterio |
|---|---|
| Logo, fotos, textos institucionales | Assets oficiales y contenido editorial aprobado; archivo/configuración de contenido o gestión existente, sin añadir un CMS por anticipado. |
| Dirección, WhatsApp, Instagram y apertura comercial | Confirmación del gimnasio; no dar por vigentes los datos de una historia antigua. |
| Tipos de membresía, clases, horarios reservables y disponibilidad | API del dominio correspondiente, cuando exista; no una réplica fija inventada en el frontend. |
| Saldo, movimientos, canjes, premios y elegibilidad | Benefits y Members por contratos autorizados a través del gateway; no cálculos decisorios en el navegador. |
| Porcentajes, vigencia y condiciones de convenios | Configuración administrada y confirmada; no porcentajes tomados de ejemplos sin revisión. |
| Mensajes y preferencias de presentación | Frontend; no almacenan API keys de integraciones ni sustituyen autorizaciones. |

El frontend usa el cliente HTTP existente y solo el gateway para operaciones del
gimnasio. Los fallos de una capacidad no esencial no dejan la home entera inutilizable.
Ni navegador ni gateway pasan a ser dueños de reglas de puntos o membresía.

## 14. Assets y datos pendientes

Logo vectorial y variantes autorizadas; paleta confirmada; fuentes/licencias; fotos
de instalaciones y actividades con recortes móvil/escritorio; autorización de
personas; clip opcional si se elige esa alternativa; dirección y apertura; WhatsApp;
planes/cupos/tarifas que puedan publicarse; convenios con vigencia y condiciones;
copy final y material Open Graph. No se requiere entregar datos personales reales
para diseñar: usar ejemplos ficticios identificados en entornos de prototipo.

Los placeholders se usan en wireframes y documentación. Producción omite o adapta
las secciones sin material confirmado; no presenta imágenes falsas, enlaces rotos,
promociones inventadas o números sin verificar como contenido definitivo.

## 15. Etapas y dependencias de frontend

| Etapa | Entrega y dependencias |
|---|---|
| FD-01 — Brief y contenido | Este documento, fuentes y pendientes; revisar con el gimnasio. Sin implementación de pantallas. |
| FD-02 — Wireframes y sistema visual | Home móvil/escritorio, navegación pública/privada, tokens, componentes y estados; validar logo y contenido. |
| FD-03 — Prototipo de movimiento | Storyboard, escena normal/estática/reducida, interacción inmediata, controles y presupuesto; elegir discos o video. |
| FD-04 — Construcción e integración | Extiende la futura etapa 20 del plan v2 sin renumerar 20A/20B/20C. Conservar React/CSS y acceso existente. Funciones privadas dependen de capacidades/roles corregidos en CB-03 a CB-07; una home pública no acredita esos flujos. |
| FD-05 — Verificación y publicación | Responsive, teclado/lector de pantalla, movimiento reducido, fallo de assets, redes lentas, SEO y rendimiento; regresión funcional y revisión de contenido antes de desplegar. |

Se puede avanzar en diseño y assets mientras sigue el backend. No ocultar su estado
real con pantallas que simulen canjes, pagos o membresías exitosos. D-70 se resuelve
antes de habilitar acceso; este brief no aprueba su mecanismo actual de seguridad.

## 16. Criterios de aceptación futuros

1. Se reconoce la dirección negro/grafito/lima y se usa material oficial validado.
2. Home, cuenta y áreas por rol tienen navegación comprensible y no eliminan flujos existentes.
3. Planes, horarios, beneficios y requisitos coinciden con fuentes aprobadas y backend.
4. Persona sin membresía activa puede ver sus puntos autorizados; no se confunden saldos y premios.
5. Copy y CTA funcionan desde el primer instante; ningún efecto bloquea navegación.
6. Escena breve, silenciosa, sin bucle automático, con versión estática y controles accesibles.
7. Movimiento reducido, ahorro de datos informado, pestaña oculta y fallo de recurso tienen comportamiento probado.
8. Teclado, foco, lectores de pantalla, contraste, móvil y zoom se verifican sobre el resultado real.
9. Se distinguen carga, vacío, proceso, confirmación, rechazo y resultado desconocido.
10. Reintentar una intención no duplica reservas/canjes; UI no declara rechazo por timeout.
11. No se exponen datos ajenos ni secretos y los roles se validan en backend.
12. Build/lint, pruebas funcionales y mediciones se registran con condiciones y límites.
13. El README de frontend explica edición de contenido, sustitución de assets y pendientes.
14. Revisión visual y del equipo antes de publicar; estos criterios no se marcan cumplidos por escribir el brief.

## 17. Fundamento verificable

Teórico EN PDF.pdf, numeración del archivo, no de una diapositiva:
p. 23 (propiedad y API de los servicios), p. 90 (gateway/BFF y límites de composición),
p. 97 (justificar capas adicionales), p. 102 (esperas y presupuestos no universales),
p. 104 (intención y clave estables; botón deshabilitado no basta), p. 110 (estados
de negocio y recuperación en frontend), p. 116 (fallback honesto), p. 118 (capas y
dependencias). No se atribuye al material una estética, WCAG o los presupuestos visuales.

Fuentes complementarias consultadas el 06/11/2026:

- W3C, control de movimiento: https://www.w3.org/WAI/WCAG22/Understanding/pause-stop-hide.html
- W3C, contraste: https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html
- web.dev, rendimiento de video: https://web.dev/learn/performance/video-performance
- web.dev, umbrales de experiencia: https://web.dev/articles/defining-core-web-vitals-thresholds

Estas fuentes complementan el diseño; no sustituyen el teórico ni certifican el sitio.
