# Registro externo, activación y acceso al Club

> **Actualización CB-03:** la divergencia D-70 se corrigió localmente aplicando
> JWT RS256 de 15 minutos, roles y bcrypt mínimo 12 del ADR-006. Pasaron pruebas
> con PostgreSQL y SMTP local reales; ver [evidencia](CB03_VALIDACION.md).
> ADR-017 y la integración de cambios siguen pendientes de revisión del equipo.
> Definir variables no equivale a aprobación ni despliegue; flags apagados por defecto.

Implementación local del 6 de noviembre de 2026. Complementa la revisión inicial
en [CLUB_BENEFICIOS_REVISION.md](CLUB_BENEFICIOS_REVISION.md). No se desplegó ni se
conectó a un proveedor de correo de producción. Contrato de acceso:
[API de autenticación v1.0.0](api/auth.v1.md).

## Qué se implementó

- Identidad en Members con correo normalizado único, contraseña con bcrypt y
  bandera `registro_externo` para cuentas nuevas. Una cuenta previa conserva
  nombre, contraseña y origen original; su vínculo externo queda registrado aparte.
- Vínculos únicos por integración e identificador externo, sin reasignaciones.
- Registro persistido e idempotente por integración/evento y por clave HTTP.
- Procesamiento reintentable Members → Benefits mediante API interna autenticada,
  sin consultar bases ajenas. Se recupera la cuenta de puntos por `alumno_id`.
- Activación con token aleatorio de 256 bits: hash para verificar, copia cifrada
  con AES-GCM para entregar por outbox, expiración de 24 horas y consumo atómico.
  La contraseña se establece únicamente si todavía no existe.
- Bienvenida después de procesar los puntos. Fallos SMTP no repiten la acreditación.
- JWT RS256 de 900 segundos, con rol persistido; el gateway verifica firma y
  claims con la clave pública. Bcrypt mínimo 12; hashes previos se actualizan
  tras un login válido. No se agregaron dependencias.
- Pantalla de activación, ingreso, saldo propio, actualización y cierre de sesión.
  El token de activación llega en el fragmento URL y se retira de la barra; no
  viaja como query a servidores. El JWT se conserva solo en memoria del navegador
  y se descarta al vencer, recibir 401 o cerrar sesión. No hay refresh ni revocación
  en v1; una copia sigue válida hasta exp, conforme al ADR-006.
- Reenvío de enlaces pendientes, sin revelar si existe el correo y sin restablecer
  una contraseña ya establecida. Rotar el enlace invalida el anterior.
- Cuota de solicitudes por integración en PostgreSQL (`max_solicitudes_minuto`,
  inicialmente 120, configurable), incluso entre réplicas. Login: 10 intentos por
  correo/minuto y 120 globales; activación: 120 globales/minuto; reenvío: uno por
  correo/minuto. Son límites operativos, no reglas de recompensas comerciales.

## Flujo de identidad

Usuario nuevo: alta sin membresía → vínculo externo → cuenta de puntos →
acreditación transaccional → correo de bienvenida → contraseña de primera vez.

Usuario existente sin vínculo previo: solicitud pendiente → correo de confirmación
→ prueba de control del correo → vínculo verificado → acreditación → confirmación
del beneficio. El correo informado por la plataforma, por sí solo, no concede acceso
a una cuenta previa. Si el vínculo ya estaba verificado, no se exige confirmarlo otra vez.

El recibo externo no informa si el correo existía. `accepted` incluye procesamiento
y verificación pendientes; `completed` confirma que terminó la vinculación y el
efecto de puntos. El correo se entrega por separado, con reintentos.

No se puede cambiar el correo de un identificador externo ya vinculado ni asignar
otro identificador de esa integración a la misma persona mediante este endpoint:
esas situaciones requieren un flujo administrativo de conciliación posterior.
Si la plataforma omitió el identificador, debe conservar el `externalUserId`
devuelto para sus operaciones siguientes.

## Endpoints externos

Base: `<gateway>/partner-api/v1`. Autenticación: `X-API-Key`. La integración debe
estar activa y tener `permite_registro=true`; cuando hay puntos, también requiere
`permite_puntos=true` y cumplir los límites. El nombre de plataforma se obtiene
de la credencial validada, no se confía en una plataforma declarada en el cuerpo.

### POST /external-register

Headers: `Content-Type: application/json`, `Idempotency-Key: estudiario-evento-0001`,
`X-API-Key: <secreto exclusivo de Estudiario>`.

```json
{
  "name": "Persona de ejemplo",
  "email": "persona@example.test",
  "externalUserId": "estudiario-001",
  "eventId": "participacion-001",
  "points": 150,
  "reason": "Participación confirmada",
  "reference": "participacion-001"
}
```

Obligatorios: nombre (1–120), correo válido (hasta 254), `eventId` (1–128,
empieza con letra o número y admite después letras, números y `._:@-`).
El identificador externo es opcional; el sistema genera uno estable, específico
de la integración. Puntos opcionales (cero por defecto, hasta 1.000.000 y los topes
del partner). Motivo opcional (`Registro externo` por defecto), 3–200 caracteres.
Referencia opcional (evento truncado a 100 caracteres por defecto), 1–100.
Cuerpo máximo de 16 KiB, sin campos desconocidos ni parámetros de consulta.

Respuesta 202:

```json
{
  "registrationId": "7d0c4a8e-2f4b-4c51-9e0e-3c2b8f1a9d10",
  "eventId": "participacion-001",
  "externalUserId": "estudiario-001",
  "status": "accepted"
}
```

Repetir con el mismo evento y contenido recupera el mismo recibo. Una nueva key
para ese evento también se reserva para impedir que luego identifique otro evento.
Cambiar el contenido o reutilizar una key para otro evento devuelve 409.
La deduplicación de la acreditación usa el mismo `eventId` que `credits`: enviar
el hecho por ambos endpoints no autoriza dos movimientos.

### GET /external-registrations/{eventId}

Autenticado y limitado a la integración del solicitante. Responde 200 con el recibo
y estado `accepted` o `completed`. Un evento ajeno o inexistente responde 404.
Consultar periódicamente, respetando la cuota y `Retry-After`; no crear otro evento
para resolver un timeout ni reenviar datos diferentes mientras espera confirmación.

Después de `completed`, usar el endpoint de saldo existente:
`GET /accounts/{externalUserId}/balance`, que exige permiso de consulta.

Errores de partners: 400 `INVALID_REQUEST`, 401 `UNAUTHORIZED`, 403 `FORBIDDEN`,
404 `ACCOUNT_NOT_FOUND`, 409 `IDEMPOTENCY_CONFLICT`, 429 `RATE_LIMITED`,
503 `UNAVAILABLE`, con RFC 7807. Los errores de transporte en el gateway pueden
ser 502 o 504; reintentar el mismo evento y key.

## Endpoints para el usuario del gimnasio

Base: `<gateway>/api/v1`, JSON. Errores RFC 7807 con `correlation_id`.

| Método y ruta | Entrada / autenticación | Respuesta |
|---|---|---|
| POST `/auth/activate` | `{"token":"<enlace>","password":"<nueva>"}` | 200 `{"status":"activated"}` |
| POST `/auth/resend-activation` | `{"email":"persona@example.test"}` | 202 `{"status":"accepted"}` |
| POST `/auth/login` | `{"email":"persona@example.test","password":"<contraseña>"}` | 200 con `access_token`, `token_type: Bearer`, `expires_in: 900` |
| POST `/auth/logout` | `Authorization: Bearer <JWT>` | 204; el cliente descarta el JWT; no revoca copias |
| GET `/users/me` | `Authorization: Bearer <JWT>` | 200 con datos propios y role; no permite elegir otra identidad |
| GET `/benefits/me` | `Authorization: Bearer <JWT>` | 200 `{"balance":150,"updatedAt":"2026-11-06T15:00:00Z"}` |

Para una vinculación de una cuenta que ya tiene contraseña, `activate` puede enviar
`password: ""`; no modifica la contraseña actual. Para una cuenta sin contraseña
se requieren al menos 12 caracteres y un máximo de 72 bytes UTF-8 (límite bcrypt).
Un enlace usado o vencido responde 401 `NO_AUTENTICADO`.
Entrada inválida: 400 `VALIDACION`; conflicto de identidad: 409 `CONFLICTO_IDENTIDAD`;
cuota agotada: 429 `LIMITE_EXCEDIDO`. Los errores internos no exponen SQL ni datos
personales. El gateway verifica el JWT localmente y descarta cabeceras de
identidad enviadas por el navegador antes de inyectar sub/role validados.

Ejemplo de login exitoso (token abreviado, no utilizable):

```json
{"access_token":"<JWT-RS256>","token_type":"Bearer","expires_in":900}
```

Sin token, firma/claims inválidos o expiración: 401 NO_AUTENTICADO. Rol no
permitido por la ruta: 403 SIN_PERMISO. Cuenta propia y logout admiten los cuatro
roles del ADR-006; otros recursos privados aún por implementar quedan restringidos
a ADMINISTRADOR hasta definir su matriz específica. La identidad nunca se toma
de un parámetro, del body o de una cabecera aportada por el cliente.

## Configuración y despliegue pendiente

El despliegue existente queda deshabilitado para estas rutas hasta configurar:

```dotenv
MEMBERS_IDENTITY_ENABLED=true
BENEFITS_EXTERNAL_REGISTRATION_ENABLED=true
GYM_INTERNAL_SERVICE_KEY=<secreto aleatorio de al menos 32 caracteres>
MEMBERS_ACTIVATION_ENCRYPTION_KEY=<otro secreto aleatorio de al menos 32 caracteres>
MEMBERS_JWT_PRIVATE_KEY=<clave privada RSA PEM de al menos 2048 bits>
GATEWAY_JWT_PUBLIC_KEY=<clave pública PEM del mismo par, formato PUBLIC KEY>
MEMBERS_PUBLIC_URL=https://<frontend>
MEMBERS_SMTP_HOST=<servidor STARTTLS>
MEMBERS_SMTP_PORT=587
MEMBERS_SMTP_FROM=<correo remitente>
MEMBERS_SMTP_USER=<usuario>
MEMBERS_SMTP_PASSWORD=<secreto>
MEMBERS_SMTP_ALLOW_PLAINTEXT=false
```

No guardar estos valores en Git ni en `VITE_*`. Gateway, Members y Benefits necesitan
la misma clave interna. La clave de cifrado es independiente y debe conservarse
para descifrar correos pendientes; no rotarla sin un procedimiento de migración.
El enlace público requiere HTTPS, excepto localhost para desarrollo.
La clave privada RSA se configura solamente en Members; la pública solamente en
el gateway. Se admite PEM multilínea o secuencias literales \n. Conservar el par
entre reinicios; la rotación v1 es coordinada y requiere nuevo login. No se publica
la privada ni se genera automáticamente al arrancar un servicio.

Members aplica migraciones 001/002 al habilitar identidad; Benefits aplica hasta
005, con permiso de registro desactivado por defecto y evento exigido por defecto
en integraciones nuevas. Las sesiones opacas del borrador dejan de autenticar;
su tabla se conserva sin uso y se requiere login nuevo. Revisar y respaldar
antes de desplegar. Si se importan usuarios históricos, deben conservar el UUID
referenciado por `cuentas.alumno_id`; no intentar conciliarlos únicamente por nombre.

Para pruebas locales puede usarse el perfil Compose `identity`, que agrega Mailpit
en `http://localhost:8025`. Requiere habilitar los dos flags, completar secretos y
poner `MEMBERS_SMTP_ALLOW_PLAINTEXT=true` solamente para ese SMTP local sin credenciales.
El correo de producción exige STARTTLS y verifica el certificado.

No se cambiaron `.env`, claves reales, Vercel, permisos GitHub ni despliegues activos.

## Garantías y límites

La operación entre dos bases es una coordinación reintentable, no una transacción
distribuida. Members conserva el comando; Benefits deduplica el evento y acredita
atómicamente. Si el proceso cae después de acreditar y antes de guardar el recibo,
el siguiente intento recupera el movimiento original. La cola de correo se guarda
en la misma transacción que el cambio que la genera. SMTP puede entregar un correo
más de una vez tras un fallo al confirmar; se usa Message-ID estable y el enlace
solo puede consumirse una vez.

Las cuotas de puntos y permisos se vuelven a verificar al procesar. Una revocación
o presupuesto agotado conserva el comando pendiente con backoff, sin borrar puntos.
La recuperación administrativa de comandos pendientes y métricas todavía requiere
el panel del Club. Los registros y tokens se guardan en Members, nunca en el ledger
de otra base. Es necesario programar la limpieza de contadores
de cuotas antiguos; no debe eliminar movimientos ni identidades de eventos.

Todavía faltan membresías, reglas permitidas por acción externa, promociones y
recompensas especiales, canjes y gestión administrativa con roles. Esta entrega
no los declara implementados.

## Validación

CB-02 verificó ledger, límites y contrato con PostgreSQL y Spectral. CB-03 pasó
100 resultados de tests/subtests y vet en pkg/gateway/members, nueve escenarios
E2E con los tres servicios, PostgreSQL y Mailpit SMTP reales, y build frontend.
No se omitieron tests. Informes reproducibles y límites en CB03_VALIDACION.md.

Se probó activación de un uso, expiración/reenvío, usuario existente con prueba
de correo sin reset, actualización bcrypt 10 a 12, JWT/roles/cabeceras falsas,
registro sin puntos, caída SMTP y pérdida de respuesta después de commit real.
La limitación anterior de Docker queda superada para estas etapas locales.
No se validó el SMTP productivo, automatización del navegador ni todo el Club.
