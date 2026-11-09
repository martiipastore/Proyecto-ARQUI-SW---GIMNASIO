# Publicar el Club de Beneficios desde una cuenta propia de Vercel

> Decisión: D-75 / [ADR-021](../adr/ADR-021-despliegue-vercel.md), que reemplaza la
> VM de Oracle (D-41). El llamado cruzado entre Benefits y Members que explica
> `members-registration` está en [ADR-022](../adr/ADR-022-registro-externo-members-benefits.md).

Esta guía despliega la capacidad del Club sin conectar el repositorio de GitHub a Vercel. La copia local se envía con Vercel CLI. El resultado es una URL HTTPS que sigue disponible aunque la computadora esté apagada.

## Componentes

`vercel.json` define cinco servicios HTTP en un proyecto: Gateway (`/partner-api/*` y `/api/*`), Benefits, Members, una segunda instancia privada de Members para recibir registros (`members-registration`) y el frontend de activación. Los servicios Go se comunican mediante *service bindings*. Vercel no permite bindings circulares, por eso Benefits llama a `members-registration`, que usa el mismo código y la misma base `members_db` sin depender de Benefits. Esa instancia no tiene ruta pública ni worker propio. Un Vercel Daemon ejecuta `members-worker` para completar registros, enviar correos y vencer membresías. Los tres `Dockerfile.*.vercel` están en la raíz porque Vercel toma la carpeta del Dockerfile como contexto de construcción y cada servicio Go necesita también `pkg/`. Las dos bases PostgreSQL y el servidor SMTP son servicios externos. Docker Compose sigue siendo el entorno local.

Vercel Services y Daemons están en beta. Daemons corre solo en producción y consume cuota de memoria mientras está activo. Antes de dejarlo encendido, revisar el uso del plan Hobby en el panel de Vercel.

## 1. Vincular el proyecto local

Desde CMD, en la raíz del repositorio:

```cmd
cd /d "<ruta-local-del-repositorio>"
vercel whoami
vercel link
```

En `vercel link`, seleccionar la cuenta propia (`matias-schaffer`) y crear un proyecto, por ejemplo `metalfitness-club`. Si ofrece conectar el repositorio de GitHub, omitir esa conexión. El archivo `.vercel/` que crea la CLI está ignorado por Git.

En **Project → Settings → Build and Deployment**, comprobar que el framework sea **Services**. La configuración de los servicios está en `vercel.json`.

## 2. Crear PostgreSQL persistente

En **Project → Storage / Marketplace**, seleccionar una integración PostgreSQL (por ejemplo Neon). Crear dos bases lógicas: una para `members` y otra para `benefits`. Pueden estar en el mismo servidor si el proveedor permite bases independientes. Guardar las URL de conexión con TLS (`sslmode=require`):

| Variable de Vercel | Valor |
| --- | --- |
| `MEMBERS_DB_DSN` | URL PostgreSQL de la base de Members |
| `BENEFITS_DB_DSN` | URL PostgreSQL de la base de Benefits |

En ambas URL se puede limitar el pool de cada instancia añadiendo `pool_max_conns=2` como parámetro de consulta. Si la URL ya tiene `?`, agregar `&pool_max_conns=2`.

Las migraciones se aplican al iniciar cada servicio. La primera ejecución necesita usuarios de base con permiso para crear tablas e índices. Las bases nuevas empiezan vacías; el alta del otro grupo se realiza después del primer despliegue.

## 3. Configurar SMTP

Contratar o usar un proveedor SMTP que permita **STARTTLS** en el puerto 587 y verificar allí la dirección de envío. El código inicia una conexión SMTP y luego negocia STARTTLS; un servidor que acepte solo TLS implícito en el puerto 465 no funciona con el adaptador actual.

Agregar `MEMBERS_SMTP_HOST`, `MEMBERS_SMTP_PORT=587`, `MEMBERS_SMTP_FROM`, `MEMBERS_SMTP_USER` y `MEMBERS_SMTP_PASSWORD` en Vercel. `MEMBERS_SMTP_ALLOW_PLAINTEXT` debe ser `false`.

## 4. Generar secretos y variables de producción

Desde la raíz, generar un archivo privado local:

```cmd
go run scripts/generate-club-secrets.go > .env.vercel-secrets
```

El archivo está ignorado por Git y excluido del envío de Vercel CLI. Contiene cuatro líneas: `GYM_INTERNAL_SERVICE_KEY`, `MEMBERS_ACTIVATION_ENCRYPTION_KEY`, `MEMBERS_JWT_PRIVATE_KEY` y `GATEWAY_JWT_PUBLIC_KEY`. Cargar sus valores en **Project → Settings → Environment Variables**, marcando los secretos como **Secret**. No copiar esas líneas a un issue, commit, chat o captura.

Agregar también las siguientes variables con destino **Production**:

| Variable | Valor |
| --- | --- |
| `PORT` | `8080` |
| `GATEWAY_HTTP_ADDR` | `:8080` |
| `BENEFITS_HTTP_ADDR` | `:8080` |
| `MEMBERS_HTTP_ADDR` | `:8080` |
| `MEMBERS_IDENTITY_ENABLED` | `true` |
| `BENEFITS_EXTERNAL_REGISTRATION_ENABLED` | `true` |
| `MEMBERS_BACKGROUND_WORKER_ENABLED` | `false` |
| `MEMBERS_WORKER_INTERVAL` | `10s` |
| `MEMBERS_PUBLIC_URL` | URL HTTPS de producción del proyecto, sin barra final |
| `GATEWAY_CORS_ORIGINS` | La misma URL HTTPS de producción |
| `MEMBERS_SMTP_ALLOW_PLAINTEXT` | `false` |

Agregar los dos DSN y los cinco valores SMTP de los pasos anteriores. Los bindings de `vercel.json` suministran automáticamente las URL entre Gateway, Benefits y Members; no crear manualmente `GATEWAY_MEMBERS_URL`, `GATEWAY_BENEFITS_URL`, `BENEFITS_MEMBERS_URL` ni `MEMBERS_BENEFITS_URL`.

El frontend usa rutas del mismo dominio en producción. No definir `VITE_API_URL` con `localhost` en Vercel.

Las variables agregadas después de un despliegue se aplican en uno nuevo. Si aún no se conoce la URL estable del proyecto, verla en **Project → Settings → Domains**, configurar `MEMBERS_PUBLIC_URL` y desplegar de nuevo.

## 5. Desplegar

Desde la raíz:

```cmd
vercel deploy --prod
```

La CLI devuelve una URL HTTPS. En **Project → Deployments** verificar que el build terminó y en **Project → Logs** comprobar que `members-worker` corre. Un despliegue Preview no ejecuta Daemons durante la beta; el registro completo debe probarse en Production.

## 6. Dar de alta al otro grupo

Después de que Benefits aplique las migraciones y `BENEFITS_DB_DSN` sea accesible desde la computadora, ejecutar desde CMD en la raíz:

```cmd
powershell -NoProfile -File scripts/provision-club-partner.ps1 -Name "Nombre del sistema consumidor"
```

El comando pide sin eco `DATABASE_URL_UNPOOLED` de `metalfitness-benefits` desde Neon. Vercel no permite recuperar por CLI el valor de `BENEFITS_DB_DSN` marcado como Secret. El comando crea un partner con permisos de registro, consulta y acreditación, y muestra su API key **una sola vez**. Enviarla por un canal privado al contacto técnico del otro grupo. No registrarla en Git, en Vercel como variable del frontend ni en documentación pública.

Si una API key queda expuesta, rotarla con `powershell -NoProfile -File scripts/provision-club-partner.ps1 -RotateId "UUID-del-partner"`. El comando vuelve a pedir la URL directa de Benefits, reemplaza el hash almacenado y muestra una clave nueva una sola vez; la anterior deja de funcionar.

Para recuperar el `externalUserId` de una prueba cuyo terminal se cerró, usar `powershell -NoProfile -File scripts/provision-club-partner.ps1 -ListUsersId "UUID-del-partner"`. Esta consulta pide la URL de Benefits y solo lista los identificadores vinculados a ese partner; no modifica la base.

## 7. Verificar la API pública

Con la URL de Production, hacer primero una solicitud sin API key:

```cmd
curl.exe -i https://metalfitness-club.vercel.app/partner-api/v2/accounts/usuario-prueba/balance
```

Debe responder `401` y JSON de problema, lo que confirma que Gateway y Benefits están accesibles. Después, con una cuenta de prueba y la API key, ejecutar el flujo documentado en [`docs/contracts/README.md`](../contracts/README.md): registro, consulta del recibo, activación desde el correo, acreditación, saldo, movimientos e idempotencia. El registro debe pasar a `completed` y llegar el correo antes de entregar la capacidad.

Para probar registro y recibo sin dejar la API key en el historial de CMD, ejecutar `powershell -NoProfile -File scripts/smoke-club-vercel.ps1`. El script pide la key nueva y un correo real sin guardarlos, crea un identificador de prueba único y consulta el recibo durante hasta 90 segundos. Luego hay que abrir el correo de activación y completar el resto del flujo con la guía del contrato.

Para probar crédito, saldo, movimientos e idempotencia con el `externalUserId` mostrado en el registro, ejecutar `powershell -NoProfile -File scripts/check-club-credit-vercel.ps1`. El script pide la API key sin mostrarla y acredita 25 puntos. Después, en el frontend del Club, pulsar **Actualizar saldo** para comprobar que se ve el mismo valor. Esta secuencia se completó en producción el 08/11/2026.

Entregar al otro grupo: URL base `https://metalfitness-club.vercel.app/partner-api/v2`, contrato OpenAPI v2, [guía de integración](../contracts/README.md), API key privada y contacto de soporte. Crear un partner distinto del de prueba para el otro grupo.

## Actualizaciones posteriores

Sin integración de Git, un `git push` no publica en Vercel. Desde la copia local, actualizar la rama aprobada y ejecutar `vercel deploy --prod` para publicar una nueva versión. El fallo de GitHub Actions por facturación no impide este flujo de Vercel CLI.

## Fuentes de la plataforma

- [Desplegar proyectos desde Vercel CLI](https://vercel.com/docs/projects/deploy-from-cli)
- [Vercel Services](https://vercel.com/kb/guide/vercel-services)
- [Vercel Daemons](https://vercel.com/docs/daemons)
- [PostgreSQL en Vercel Marketplace](https://vercel.com/docs/postgres)
- [Variables de entorno](https://vercel.com/docs/environment-variables)
