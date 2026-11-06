# API de acceso del gimnasio — v1.0.0

Preparada y probada localmente el 06/11/2026, CB-03. Base pública: `<gateway>/api/v1`.
No es la API de partners, que conserva su contrato separado. Se requiere habilitar
identidad y configurar el par RSA antes de desplegar; flags apagados por defecto.

## Endpoints

| Método y ruta | Autenticación | Solicitud y respuesta |
|---|---|---|
| POST /auth/activate | Token de activación en body | token y password; 200 con status=activated |
| POST /auth/resend-activation | Público, con cuota | email; siempre 202 status=accepted si formato válido, sin revelar existencia |
| POST /auth/login | Público, con cuota | email y password; 200 con JWT RS256 |
| POST /auth/logout | Bearer JWT válido | Sin body; 204, cliente descarta su token |
| GET /users/me | Bearer JWT válido | 200 con datos del actor verificado |
| GET /benefits/me | Bearer JWT válido | 200 con balance y updatedAt; 404 si la cuenta de puntos aún no está disponible |

Content-Type application/json para cuerpos, máximo 16 KiB, sin campos ajenos ni
query strings. Email normalizado en backend, máximo 254 caracteres. Activación:
token de 43 caracteres de alta entropía; contraseña de al menos 12 caracteres y
máximo 72 bytes UTF-8. Una cuenta que ya tenía contraseña puede confirmar con
password vacío: su contraseña y rol se conservan. Login no crea ni vincula cuentas.

Los JWT contienen sub UUID, role (ADMINISTRADOR/PROFESOR/NUTRICIONISTA/ALUMNO),
iss=gym-members, aud=gym-api, iat y exp, con máximo 900 s. Solo RS256. JWT verifica
en gateway; Members recibe identidad propagada y credencial interna, sin volver a
decodificar JWT. Los clientes nunca envían X-User-* ni X-Internal-* como identidad.

## Ejemplos

POST /auth/login:

```json
{"email":"persona@example.test","password":"<contraseña>"}
```

200 (token de ejemplo abreviado):

```json
{"access_token":"<JWT-RS256>","token_type":"Bearer","expires_in":900}
```

GET /users/me con Authorization: Bearer <JWT>:

```json
{"id":"11111111-1111-4111-8111-111111111111","name":"Persona","email":"persona@example.test","external_registration":true,"role":"ALUMNO"}
```

GET /benefits/me con la misma autenticación:

```json
{"balance":150,"updatedAt":"2026-11-06T15:00:00Z"}
```

POST /auth/activate:

```json
{"token":"<token de activación de un solo uso>","password":"<contraseña nueva>"}
```

200:

```json
{"status":"activated"}
```

## Errores y recuperación

Errores RFC 7807 application/problem+json; correlation_id identifica la solicitud,
sin secretos ni datos personales en el mensaje.

| HTTP/code | Causa y acción |
|---|---|
| 400 VALIDACION | Formato o contraseña fuera de límites; corregir datos |
| 401 NO_AUTENTICADO | Login inválido, JWT inválido/vencido o activación usada/vencida; solicitar acceso o reenviar activación según el flujo |
| 403 SIN_PERMISO | Rol no autorizado; no repetir como otro usuario |
| 409 CONFLICTO_IDENTIDAD | Evento/vínculo incompatible en el registro interno; mantener intención y revisar el caso |
| 429 LIMITE_EXCEDIDO | Cuota agotada; Retry-After: 60 |
| 404 CUENTA_NO_ENCONTRADA | La cuenta de puntos todavía no está disponible; consultar el recibo externo correspondiente |
| 500/502/503/504 | Falla técnica o dependencia; conservar contexto y recuperar resultado, sin inventar confirmación o rechazo |

Ejemplo de token vencido:

```json
{"type":"https://gym.local/problems/no-autenticado","title":"No autorizado","status":401,"detail":"Credencial inválida o vencida.","instance":"/api/v1/benefits/me","code":"NO_AUTENTICADO","correlation_id":"3c5e7a9b-1d2f-4a6b-8c0d-2e4f6a8b0c1d"}
```

La activación es atómica y de un uso. Si se pierde su respuesta, un 401 posterior
puede indicar consumo previo; intentar login o solicitar un nuevo enlace, sin
restablecer una contraseña existente. El registro externo usa evento/key durables;
consultar su recibo antes de crear una intención distinta. El saldo de un recibo o
acreditación repetidos puede ser histórico: consultar /benefits/me para actualizarlo.

El frontend mantiene el JWT solo en memoria, lo elimina al vencer/recibir 401 y
siempre permite cerrar acceso local. Logout v1 no revoca copias del JWT: permanecen
válidas hasta exp. Sin refresh tokens; se requiere nuevo login. Una membresía
inactiva no invalida la cuenta ni borra sus puntos.

## Límites y referencias

Cuenta propia/logout admiten los cuatro roles válidos. Otros recursos privados
no implementados quedan restringidos a ADMINISTRADOR hasta definir su matriz;
esto no anuncia endpoints de administración ni permisos finos ya operativos.
Cuotas iniciales: login 10/correo/min y 120 globales/min; activación 120/min;
reenvío 1/correo/min. Parámetros operativos locales, revisión pendiente para uso real.

[Registro externo y configuración](../CLUB_REGISTRO_EXTERNO.md),
[contrato de partners](../contracts/README.md),
[evidencia CB-03](../CB03_VALIDACION.md),
[ADR-006 aceptado](../adr/ADR-006-seguridad.md) y
[ADR-017 propuesto](../adr/ADR-017-registro-identidad-activacion.md).

CHANGELOG 1.0.0: acceso JWT 900 s conforme a ADR-006. Sustituye el borrador local
no desplegado de sesiones opacas; no cambia la versión de la API de partners.
