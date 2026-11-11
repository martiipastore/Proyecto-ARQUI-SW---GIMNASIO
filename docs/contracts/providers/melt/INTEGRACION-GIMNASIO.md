# Integración de Gimnasio con MELT

URL base HTTPS: `https://melt-gateway-prueba.onrender.com/api/v1`.
Los nombres de los servicios conservan el sufijo prueba; este es el entorno de entrega del proyecto académico. No usar las direcciones antiguas del equipo.

## Responsabilidades y autenticación
Gimnasio determina la cantidad. MELT no consulta planes ni membresías. El backend consumidor llama `POST /beneficios/acreditaciones` (alias `/beneficios/acreditar-puntos`) con `Content-Type: application/json` y `X-Partner-Key: <clave compartida por privado>`. No incluir la clave en frontend, enlaces ni repositorios.

## Solicitudes
Por cuenta existente, usar el ID real que devuelve Auth:
```json
{"usuarioId":"ID_REAL_MELT","puntos":500,"referencia":"gym-operacion-001","descripcion":"Puntos otorgados por Gimnasio"}
```
Por email, incluso si todavía no existe una cuenta:
```json
{"email":"persona@example.com","puntos":500,"referencia":"gym-operacion-002","descripcion":"Puntos otorgados por Gimnasio"}
```
Enviar exactamente un destinatario (usuarioId o email). referencia y solicitudId son nombres alternativos: enviar uno solo. La referencia identifica globalmente la operación y debe conservarse para reintentos. Descripcion es opcional y admite hasta 500 caracteres. Puntos debe ser entero positivo (máximo 9007199254740991). Referencia y usuarioId admiten 1–128 caracteres sin espacios extremos. Los nombres de campos son sensibles a mayúsculas; PUNTOS no es válido.

## Respuestas
- 201: acreditación realizada. Devuelve solicitudId, usuarioId, puntos, saldoPuntos, tipo, creadoEn y estado.
- 200: la misma operación ya fue acreditada; devuelve el movimiento original sin sumar otra vez. saldoPuntos corresponde al saldo después de ESA operación.
- 202: pendiente. Devuelve solicitudId, estado pendiente, puntos, puntosAcreditados 0, registroUrl y venceEn.

Ejemplo ilustrativo de acreditación (los IDs y fechas dependen de la ejecución):
```json
{"solicitudId":"gym-operacion-001","usuarioId":"ID_REAL_MELT","puntos":500,"saldoPuntos":500,"tipo":"acreditacion_externa","estado":"acreditada","creadoEn":"2026-11-11T05:00:00Z"}
```

Cuando devuelve 202, Gimnasio entrega registroUrl privadamente al destinatario cuyo email verificó. MELT no envía el correo. La persona abre el enlace y se registra o inicia sesión con el mismo email; MELT confirma los puntos automáticamente. El enlace vence en siete días. Repetir la misma solicitud de partner renueva un enlace pendiente vencido sin perder los puntos. No publicar enlaces ni registrarlos en logs.

Una cuenta existente sin email verificado también recibe enlace antes de su primera acreditación por email. Por usuarioId, una cuenta activa recibe acreditación directa.

## Errores
| HTTP | Significado | Acción |
|---|---|---|
| 400 | Datos inválidos, propiedades desconocidas o puntos inválidos | Corregir el JSON |
| 401 | Clave ausente/incorrecta | Revisar X-Partner-Key |
| 404 | UsuarioId inexistente/inactivo | Obtener ID válido; por email se admite pendiente |
| 409 | Referencia ya usada con otros datos o cuenta email inactiva | No reutilizar referencias para otra operación |
| 415 | Content-Type incorrecto o ausente | Enviar application/json |
| 422 | Saldo fuera de rango | Revisar operación, no se acredita parcialmente |
| 503 | Dependencia/configuración no disponible o resultado incierto | Reintentar con la misma referencia y datos |

Un timeout no demuestra que la operación falló: conservar la referencia al reintentar. Los servicios gratuitos pueden tardar en arrancar tras inactividad; precalentar Auth, Beneficios y Gateway mediante sus /health antes de una demostración. Un fallo al primer intento puede requerir repetir después de que estén activos.

## Consulta e invitaciones
- `GET /beneficios/mis-puntos`: Bearer de un usuario con rol cliente. Devuelve saldo actual e historial de su cuenta. Ignora headers de identidad falsificados.
- `POST /beneficios/invitaciones/consultar`: JSON {"token":"TOKEN_DEL_ENLACE"}; sin sesión; permite mostrar el destinatario y puntos.
- `POST /beneficios/invitaciones/confirmar`: mismo JSON y Bearer del destinatario. 201 primera acreditación, 200 repetición, 403 destinatario distinto, 404 token inválido, 410 token pendiente vencido.
- `POST /auth/registro`: nombre, email, password y opcional invitacionToken. 201 cuenta creada; puede informar acreditacion pendiente_reintento si la entrega necesita recuperación.
- `POST /auth/login`: email y password; devuelve token para mis-puntos.
Las rutas /internal no son accesibles por Gateway.

## Ejemplo para backend
Ejecutar `python scripts/ejemplo-gimnasio.py` con las variables MELT_PARTNER_KEY, MELT_EMAIL, MELT_REFERENCIA y MELT_PUNTOS. Guardar la referencia en el sistema consumidor antes del primer envío. El ejemplo no imprime claves ni enlaces completos. El backend debe entregar registroUrl mediante su propio canal privado.

Contrato completo: [OpenAPI](contracts/acreditar-puntos-v1.openapi.yaml). Configuración técnica: [manual](ACREDITACION-PUNTOS.md).
