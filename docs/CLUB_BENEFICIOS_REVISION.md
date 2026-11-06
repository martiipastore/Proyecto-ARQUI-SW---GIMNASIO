# Revisión del Club de Beneficios — 6 de noviembre de 2026

> Este documento conserva el diagnóstico inicial. Estado posterior vigente:
> [registro externo local y brecha D-70](CLUB_REGISTRO_EXTERNO.md),
> [CB-02 validada con PostgreSQL y Spectral](CB02_VALIDACION.md) y
> [plan general](PLAN_CLUB_BENEFICIOS.md). Las menciones siguientes a falta de
> Docker/usuarios/cuotas describen aquella iteración, no su estado actual.

Actualización posterior: [registro externo, activación y acceso implementados](CLUB_REGISTRO_EXTERNO.md).
Los puntos pendientes de identidad de esta revisión inicial se resuelven en ese
documento; membresías, recompensas, canjes y panel administrativo continúan pendientes.

## Alcance comprobado

Esta revisión corresponde al código, no al catálogo de funcionalidades planeadas.
El pedido del equipo amplía y corrige el alcance anterior del SPEC. Esta implementación
conserva los límites de microservicios y no accede a bases ajenas.

| Requisito | Estado comprobado |
|---|---|
| Ledger, saldo no negativo, historial inmutable | Existía; conservado |
| Claves individuales almacenadas como SHA-256 | Existía; conservado |
| Revocación y estado de integración en backend | Existía mediante datos en PostgreSQL |
| Consulta de saldo e historial de cuentas vinculadas | Existía; conservado |
| Idempotencia de solicitudes y rollback | Existía; conservado |
| Permisos individuales consulta/puntos | Implementado en esta revisión |
| Límites por operación y presupuesto diario | Implementado en esta revisión |
| Deducción de eventos duplicados con distinta clave HTTP | Implementado con `eventId` |
| Registro externo y vinculación segura por identidad verificada | Pendiente |
| Activación de contraseña y bienvenida por correo | Pendiente |
| Membresías, vigencia y promociones configurables | Pendiente |
| Recompensas especiales, descuentos, períodos gratuitos | Pendiente |
| Canjes, débitos y ajustes administrativos | Pendiente; no existen en el router actual |
| Panel de administración del Club y gestión de credenciales | Pendiente |
| Límites de solicitudes y auditoría de cada consulta/rechazo | Pendiente |

Members tiene solamente infraestructura y salud; no existe todavía el modelo de
usuarios ni autenticación de gimnasio. El frontend muestra salud del sistema.
El contrato histórico describe también operaciones todavía no implementadas.
No debe presentarse ese documento como evidencia de endpoints ejecutables.

## Cambios aplicados

Migración `003_politicas_integraciones.sql`, sin editar migraciones previas:

- `permite_consulta`, `permite_puntos`, `max_puntos_operacion`, `max_puntos_dia`
  en `partners`. Se conservan los permisos y el máximo técnico anteriores;
  el presupuesto diario es opcional, no se inventa una regla comercial.
- `eventos_acreditados`: identidad única por integración, huella de contenido,
  movimiento y saldo original. Es inmutable y no caduca con las respuestas HTTP.
- Índice para sumar el consumo diario por integración.

El presupuesto usa días de Argentina y un bloqueo transaccional por integración:
solicitudes simultáneas sobre cuentas distintas no pueden consumir el mismo saldo
de presupuesto. El límite y permiso se verifican en backend. Se rechazan consultas
e historial si se retira el permiso de consulta. El replay también verifica permiso
de acreditación y vinculación vigentes, sin volver a consumir presupuesto.

`eventId` es opcional para conservar clientes actuales. Para integraciones nuevas,
debe enviarse siempre, además de `Idempotency-Key`. Sin `eventId`, cambiar la key
continúa representando una nueva operación. Las huellas de clientes anteriores se
conservan. Un evento existente enviado con datos diferentes responde 409.

Actualización CB-02: migración 005 convierte esa exigencia en validación real
mediante requiere_evento: false para partners anteriores, true por defecto en
altas nuevas. Omitirlo si está activo devuelve 400 sin crédito. Reglas por acción
y panel siguen pendientes; ver documentación vigente enlazada arriba.

Se limita a 16 KiB el cuerpo de acreditación antes de leerlo en memoria.

Los puntos no dependen de membresías en el ledger actual. Ningún cambio de esta
revisión agrega expiración o reinicio de saldo. Los escenarios de vencimiento y
renovación completos requieren implementar Members; no se declaran probados.

## API realmente ejecutable

Todas las rutas externas empiezan con `/partner-api/v1` en el gateway. Requieren
`X-API-Key`, integración activa y vinculación previa. La plataforma no puede
declarar su identidad mediante el cuerpo ni consultar cuentas de otra integración.

### POST /partner-api/v1/accounts/{externalUserId}/credits

Headers: `Content-Type: application/json`, `X-API-Key: <secreto>`,
`Idempotency-Key: evento-estudiario-0001`.

```json
{
  "points": 150,
  "reason": "Participación confirmada",
  "reference": "participacion-0001",
  "eventId": "participacion-0001"
}
```

`points`: entero de 1 a 1.000.000, además del máximo configurado y presupuesto.
`reason`: 3–200 caracteres. `reference`: 1–100. `eventId`: 1–128 si se envía.
`externalUserId`: 1–64 caracteres alfanuméricos o `._:@-`.
La key admite 8–128 caracteres alfanuméricos o `._:-`.
Se rechazan campos desconocidos, JSON adicional y parámetros de consulta.

Respuesta 201 (también al repetir):

```json
{
  "movementId": "7d0c4a8e-2f4b-4c51-9e0e-3c2b8f1a9d10",
  "externalUserId": "alumno-1",
  "type": "CREDIT",
  "points": 150,
  "reason": "Participación confirmada",
  "reference": "participacion-0001",
  "occurredAt": "2026-11-06T15:00:00Z",
  "balance": 150
}
```

`Idempotent-Replayed: true` indica repetición. El saldo de esa respuesta es el
original, no necesariamente el saldo actual; consultar `balance` para actualizarlo.
Reintentar errores de conexión con la misma key y el mismo evento. No rotar
identificadores para resolver un timeout.

### GET /partner-api/v1/accounts/{externalUserId}/balance

Requiere permiso de consulta; sin cuerpo ni parámetros. Respuesta 200:

```json
{"externalUserId":"alumno-1","balance":150,"updatedAt":"2026-11-06T15:00:00Z"}
```

### GET /partner-api/v1/accounts/{externalUserId}/movements?page=1&size=20

Requiere permiso de consulta. `page`: 1–100000; `size`: 1–100.
Devuelve `items`, `page`, `size`, `totalItems`, `totalPages`. El historial que ve
un partner incluye solamente sus movimientos; el saldo es compartido por cuenta.

### Errores

Formato RFC 7807, sin mensajes SQL ni datos personales:

| HTTP | code | Significado |
|---|---|---|
| 400 | INVALID_REQUEST | Formato, tamaño o límite de puntos inválido |
| 401 | UNAUTHORIZED | Key inexistente o revocada |
| 403 | FORBIDDEN | Integración inactiva o permiso retirado |
| 404 | ACCOUNT_NOT_FOUND | Cuenta no vinculada |
| 409 | IDEMPOTENCY_CONFLICT | Key o evento reutilizado con otro contenido |
| 409 | IDEMPOTENCY_IN_PROGRESS | Solicitud con esa key todavía en ejecución |
| 503 | UNAVAILABLE | Persistencia temporalmente indisponible |

## Migración y operación

Antes de desplegar el binario nuevo, aplicar `go run ./cmd/migrate` desde
`services/benefits-service`, con `BENEFITS_DB_DSN` en el entorno. No se ejecutó
contra una base del usuario ni producción. Respaldar y revisar la migración antes
de desplegar. No bajar un binario antiguo si ya se usan las políticas nuevas.

Los administradores con acceso a la base pueden configurar los cuatro campos
de políticas de cada partner. No se agregó un endpoint administrativo público
sin autenticación: falta primero implementar autenticación y roles en el gateway.
Desactivar `activo` revoca acceso; sustituir `api_key_hash` por el SHA-256 de una
nueva clave aleatoria revoca la anterior. Nunca almacenar el texto de la clave.

## Trabajo pendiente para el objetivo integral

1. Members: usuarios únicos por correo normalizado; origen externo y vínculos
   únicos `(integración, externalUserId)`. Un correo afirmado por una plataforma
   no prueba control de una cuenta existente: exigir confirmación por correo o
   sesión de usuario para vincularla y no permitir apropiación de cuentas.
2. Activación: token aleatorio con hash persistido, expiración y consumo atómico;
   contraseña con hash apropiado; outbox para bienvenida reintentable sin perder
   la operación si SMTP falla. No enviar contraseña por correo.
3. Saga de registro entre Members y Benefits, con evento único persistente:
   reintentos deben recuperar la misma identidad y completar la acreditación.
4. Membresías mensual/trimestral/anual/clases en Members, independientes de cuenta
   de beneficios; reglas y recompensas en Benefits, aplicadas por API/eventos.
5. Canjes y ajustes: extender ledger firmado y su trigger de conciliación juntos;
   no simular débitos guardándolos en otra tabla fuera del saldo.
6. Autenticación de administración, panel y auditoría de políticas/credenciales;
   cuotas de solicitudes, reglas por evento y promociones, no solamente un tope.

## GitHub, despliegue y secretos

`origin` apunta a `martiipastore/Proyecto-ARQUI-SW---GIMNASIO` en GitHub. Hay CI
para Go, frontend y contrato; no hay workflow de despliegue ni archivo de Vercel
en el repositorio examinado. `.env` no está versionado. No se consultaron valores
de secretos. La configuración real de Vercel y sus colaboradores no se verificó
por no disponer de conexión a esa cuenta.

La infraestructura existente es Docker Compose, con servicios Go persistentes,
PostgreSQL, MongoDB, RabbitMQ, Redis y OpenSearch. Publicar la SPA no publica ese
backend. El frontend necesita `VITE_API_URL`, una URL pública del gateway; toda
variable `VITE_*` es pública. DSN, claves de integraciones y futuras credenciales
SMTP/JWT deben quedar exclusivamente en el backend. Variables actuales de
infraestructura: ver `.env.example` y `deploy/docker-compose.yml`.

Antes de cambiar producción se requiere identificar el proyecto Vercel, repositorio
y rama conectados, dominios, entorno de preview/producción y host del backend.
Agregar colaboradores requiere revisar permisos del repositorio y del proyecto
en las cuentas correspondientes. No se modificaron permisos ni despliegues.

## Pruebas

Se conservan las pruebas existentes de saldo, revocación, aislamiento, rollback,
concurrencia e idempotencia. Se añaden pruebas de límites en dominio y PostgreSQL
para permisos, presupuesto y evento repetido con otra key. Los tests PostgreSQL
usan testcontainers y se omiten cuando Docker no está disponible. No equivalen a
verificar los diez escenarios integrales del pedido, cuyos flujos faltantes se
enumeran arriba.

Resultado local: `go test ./...` de Benefits finalizó sin fallos; las pruebas
PostgreSQL fueron omitidas por falta de Docker. Se redirigió `GOCACHE` a una
carpeta temporal porque la caché global no admite escritura en este entorno.
El lint Spectral configurado en Makefile también requiere Docker y no se ejecutó.
