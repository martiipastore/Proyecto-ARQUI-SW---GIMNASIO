# Adenda funcional del Club de Beneficios — SPEC 0.11

Fecha: 6 de noviembre de 2026. Requisitos autorizados por el pedido del equipo;
cambio preparado localmente para revisión por otro integrante y PR conforme a
CONTRIBUTING. No certifica implementación ni publicación del SPEC.

Complementa [SPEC.md](../SPEC.md), D-63 a D-69 del registro y el
[plan CB](PLAN_CLUB_BENEFICIOS.md). En el Club prevalece sobre los textos anteriores
que excluían personas externas, prohibían registrar/vincular por API o negaban
límites comerciales (S-11, HU-10, RN-26 y decisiones D-06/D-08/D-19/D-28).
No altera las reglas de reserva, asistencia, entrenamiento o nutrición.

## Historias ampliadas

| ID | Historia y aceptación funcional | Reglas |
|---|---|---|
| HU-CB-01 | Como integración autorizada, quiero solicitar alta o vinculación con nombre, correo, origen, identificador externo opcional y puntos cuando corresponda. Una persona nueva obtiene una cuenta sin membresía; una existente conserva su cuenta y no se vincula sin identificación segura. | RN-CB-01/03/04/12 |
| HU-CB-02 | Como usuario externo nuevo, quiero recibir bienvenida, puntos acreditados e instrucciones para acceder mediante activación de un solo uso. La primera contraseña se configura una vez; una vinculación no cambia una contraseña previa. | RN-CB-12/13 |
| HU-CB-03 | Como usuario, quiero conservar puntos e historial aunque mi membresía venza, se cancele, se renueve o cambie. Se distingue cuenta de membresía y de elegibilidad. | RN-CB-01/02/08 |
| HU-CB-04 | Como integración, quiero confirmar una acción autorizada que genera puntos o una recompensa especial, con identificación única y reglas del gimnasio. | RN-CB-04/05/06/09/10 |
| HU-CB-05 | Como administrador, quiero configurar beneficios por tipo/estado de membresía, vigencia y restricciones, sin porcentajes o promociones fijos inventados. | RN-CB-08/09/10/11 |
| HU-CB-06 | Como usuario, quiero canjear puntos y consultar inmediatamente el saldo correcto. Los ajustes administrativos requieren permiso y motivo. | RN-CB-02/07/11 |
| HU-CB-07 | Como administrador, quiero buscar usuarios, ver origen, membresías, saldo e historial y gestionar catálogo, promociones, integraciones, credenciales, permisos, límites y reglas. | RN-CB-03/04/11 |
| HU-CB-08 | Como equipo, quiero contratos, ejemplos, errores y pruebas reproducibles, y conocer la configuración de despliegue antes de modificar producción o permisos externos. | RN-CB-14/15 |

## Reglas del Club

| ID | Regla |
|---|---|
| RN-CB-01 | Una cuenta y sus puntos pueden existir sin membresía ni asistencia previa. Cuenta, saldo y membresía son conceptos independientes. |
| RN-CB-02 | Vencimiento, inactividad, cancelación, renovación o cambio de membresía no borran ni reinician saldo ni movimientos. La ausencia histórica de vencimiento de puntos de D-13 se conserva. |
| RN-CB-03 | Cada integración tiene identidad, credencial propia, estado y permisos comprobados por el backend. Activación, desactivación, emisión, revocación y renovación son administrativas. Credenciales no públicas ni almacenadas en texto plano. |
| RN-CB-04 | Un evento solo concede efectos permitidos por las reglas y límites configurados para esa integración y acción. Registrar integración no equivale a autorizar puntos, consultas o recompensas ilimitados. |
| RN-CB-05 | Se conservan integración, motivo y evento de cada otorgamiento. Repetir un mismo evento no repite el efecto aunque cambie la clave de solicitud; usarlo con contenido incompatible produce conflicto. |
| RN-CB-06 | La idempotencia de la solicitud y la identidad del evento son diferentes. Se conserva la retención mínima contractual de respuestas; eliminar una respuesta no debe habilitar un segundo otorgamiento por el mismo hecho. |
| RN-CB-07 | Cambios de saldo son atómicos, auditados y seguros ante concurrencia. Canjes o ajustes no pueden dejar saldo negativo ni perder historial. Un fallo de conexión no debe duplicar ni dejar un efecto sin recuperación. |
| RN-CB-08 | Contemplar mensual, trimestral, anual y por clases, vigencia y estado identificables. Duraciones/cupos por clases y condiciones comerciales se administran, sin inventar precios o cantidades definitivos. |
| RN-CB-09 | Elegibilidad de beneficios se configura por membresía, estado, vigencia, condiciones y restricciones pertinentes. Una promoción no modifica por sí sola la propiedad ni conservación del saldo. |
| RN-CB-10 | Diferenciar acreditar puntos, otorgar recompensa, aplicar descuento y otorgar período promocional de membresía. La confirmación de una integración solo habilita el efecto configurado y autorizado. |
| RN-CB-11 | La gestión requiere permisos administrativos reales. Consultas de usuario e integración solo acceden a recursos propios o vinculados autorizados. Ajustes y cambios sensibles conservan actor y justificación. |
| RN-CB-12 | Correo normalizado e identidad externa se vinculan sin duplicados. La afirmación de un correo por un tercero no prueba control de una cuenta existente. Guardar origen externo sin sobrescribir el origen de una cuenta previa. |
| RN-CB-13 | No enviar contraseñas en texto plano. Activación de un solo uso, con vencimiento, configura la contraseña inicial una vez. Reintentar correo o registro no reinicia contraseñas ni repite puntos. |
| RN-CB-14 | Publicar documentación de métodos, autenticación, parámetros, validaciones, JSON, errores y reintentos; probar los diez escenarios E2E-CB del plan, distinguiendo pruebas ejecutadas de omitidas. |
| RN-CB-15 | Verificar repositorio, ramas/despliegues, colaboradores y variables con acceso real. No exponer secretos en Git o frontend ni cambiar producción, permisos ajenos o despliegues sin informar previamente las modificaciones necesarias. |

## Trazabilidad y alcance de la revisión

HU-CB-01/07 amplían HU-10 y RN-26; RN-CB-02/07 conservan las invariantes de RN-22
y amplían los recorridos de RN-23/25. RN-CB-05/06 amplían RN-27; HU-CB-03/05
amplían HU-03/09 y RN-24. Los identificadores previos no se renumeran ni se borran.
E2E-CB-01 a E2E-CB-10 y su situación real están en PLAN_CLUB_BENEFICIOS.md.

Los mecanismos, algoritmos, parámetros de autenticación y diseño de almacenamiento
se justifican en ADR separados; no se convierten en requisitos funcionales por
haber sido elegidos en una implementación. D-70 conserva la brecha detectada y su
corrección local CB-03 conforme al ADR-006; la revisión del equipo sigue pendiente.
