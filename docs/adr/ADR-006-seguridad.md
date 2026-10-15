# ADR-006 — Autenticación, autorización y frontera de confianza

| Campo | Valor |
|---|---|
| **Estado** | Aceptado |
| **Fecha** | 2026-10-08 |
| **Decisión del TP** | D6 |
| **Autores** | Equipo del TPI |
| **Relacionado con** | SPEC RN-26, RN-27, RN-30 y RN-34; [ADR-001](ADR-001-limites-de-servicios.md); [ADR-007](ADR-007-cache-autorizacion-disponibilidad.md); [ADR-008](ADR-008-contrato-propio.md) |

## Contexto

METALFITNESS tiene dos clases de clientes externos: usuarios del gimnasio mediante la SPA y sistemas partners mediante la API de beneficios. Los servicios también se llaman entre sí para consultar membresías, asignaciones y datos de alumnos.

El sistema necesita un punto de entrada controlado sin trasladar todas las decisiones de permisos al gateway. El gateway conoce credenciales y rutas, pero cada servicio conoce la propiedad de sus recursos y las reglas del dominio. Además, el compose de desarrollo publica los puertos internos en `127.0.0.1` para diagnóstico, por lo que ese acceso local no puede considerarse equivalente al borde seguro de producción.

## Alternativas consideradas

### Alternativa A — Autenticación y autorización completas en el gateway

Descartada. El gateway tendría que consultar o replicar datos de todos los dominios para decidir si una reserva, un plan o una cuenta pertenecen al actor. Eso acopla el borde a reglas que deben permanecer en los servicios dueños.

### Alternativa B — Cada servicio valida credenciales externas

Descartada para v1. Obliga a repetir validación JWT, políticas de entrada y rate limiting en todos los servicios. También amplía la superficie pública y dificulta aplicar reglas uniformes.

### Alternativa C — Gateway para autenticación y autorización gruesa, servicios para autorización fina (elegida)

El gateway verifica la credencial externa, elimina cualquier identidad aportada por el cliente, inyecta la identidad verificada y filtra roles por ruta. Cada servicio decide el acceso al recurso usando sus propios datos y, cuando corresponde, datos vigentes del servicio dueño.

La identidad propagada se confía por aislamiento de red en esta versión. mTLS o una firma entre procesos aportarían defensa adicional, pero su operación excede el alcance del TP.

## Decisión

### Frontera de red y transporte

- En producción, Traefik es el único puerto expuesto a Internet: sirve la SPA y enruta las APIs al gateway.
- Servicios, bases, RabbitMQ, Redis, OpenSearch y observabilidad quedan en una red privada sin puertos publicados en el host.
- Las rutas `/internal/v1/**` solo se usan dentro de esa red. El gateway rechaza cualquier ruta que contenga un segmento `internal` después de normalizarla.
- El compose local puede publicar servicios en `127.0.0.1` para health checks y diagnóstico. Ese acceso permite omitir el gateway y no se considera una prueba válida de seguridad.
- CORS limita qué navegadores pueden invocar el gateway, pero no autentica ni autoriza solicitudes.

### Usuarios del gimnasio

Members autentica email y contraseña, almacenada con bcrypt de costo mínimo 12. Emite un JWT firmado con RS256 que contiene:

- `sub`: identificador del usuario;
- `role`: `ADMINISTRADOR`, `PROFESOR`, `NUTRICIONISTA` o `ALUMNO`;
- `iss`: `gym-members`;
- `aud`: `gym-api`;
- `iat` y `exp`, con una vida máxima de 15 minutos.

El gateway valida firma, algoritmo, expiración, emisor y audiencia. Antes elimina `X-User-Id`, `X-User-Role`, `X-Partner-Id` y cualquier `X-Internal-*` recibida del cliente. Solo después inyecta `X-User-Id` y `X-User-Role` derivados del token. Para partners reenvía `X-API-Key`; benefits valida la clave y deriva internamente la identidad del partner, sin confiar en `X-Partner-Id`.

No hay refresh token ni lista de revocación en v1. Una baja o cambio de rol puede conservar claims anteriores hasta 15 minutos. Las operaciones que dependen de membresía o asignación vigente consultan al dueño del dato y fallan cerradas según ADR-007; otros controles de estado se aplican cuando lo exige el caso de uso.

### Autorización

El gateway aplica autorización gruesa: una ruta declara los roles que pueden alcanzarla. Cada servicio aplica autorización fina antes de ejecutar el caso de uso:

- identidad propia del alumno;
- responsabilidad sobre una clase;
- asignación profesional–alumno vigente;
- propiedad de una reserva, plan, medición, consulta o cuenta;
- rol administrativo para operaciones de gestión.

La identidad nunca se toma del body ni de parámetros enviados por el cliente. Un identificador de recurso puede llegar en la URL o body, pero se compara con la identidad verificada y con los datos del dominio.

### Partners

Cada partner recibe una API key aleatoria de alta entropía, visible una sola vez. Benefits guarda SHA-256 de la clave, nunca su valor original. El gateway exige el header `X-API-Key` y limita solicitudes; benefits realiza la autenticación real en cada operación y comprueba:

- que el hash corresponde a una credencial vigente;
- que el partner está activo;
- que `externalUserId` está vinculado a ese partner;
- que la operación y los datos solicitados están dentro del contrato v1.

Una clave revocada deja de autorizar la siguiente solicitud. No se cachea su validez. Todas las operaciones de escritura mantienen además la idempotencia definida por ADR-008.

### Secretos y datos sensibles

- Claves privadas, contraseñas de infraestructura y API keys se inyectan mediante variables de entorno o secretos del entorno de despliegue.
- `.env` no se versiona; `.env.example` solo documenta nombres y valores seguros de desarrollo.
- No se registran contraseñas, JWT, API keys, DNI, emails ni cuerpos completos de solicitudes.
- Las respuestas de salud y estado público no incluyen DSN, errores internos ni secretos.

## Consecuencias

### Positivas

- Existe un único borde para TLS, autenticación externa, limpieza de cabeceras y rate limiting.
- Las reglas de acceso permanecen junto al servicio dueño del dato.
- Los partners no comparten el modelo de usuarios ni los roles del gimnasio.
- Los servicios no necesitan conocer claves privadas ni repetir el procesamiento de JWT.

### Negativas y riesgos aceptados

- La red privada es una frontera de confianza: un proceso interno comprometido puede falsificar cabeceras.
- Los puertos loopback de desarrollo permiten llamadas directas sin las garantías del gateway.
- Una baja o cambio de rol puede tardar hasta 15 minutos en reflejarse en los claims del JWT.
- El gateway es un punto único de entrada y debe mantenerse disponible.

Se acepta este riesgo para v1 por el alcance académico y el despliegue en una sola VM. mTLS o credenciales por servicio quedan como evolución si el sistema se distribuye entre hosts o aumenta su exposición.

## Cómo se verificará

- Una solicitud externa que envía cabeceras `X-User-*`, `X-Partner-Id` o `X-Internal-*` no logra conservarlas después del middleware de limpieza.
- Un JWT con firma, algoritmo, emisor, audiencia o expiración inválidos recibe `401`.
- Un JWT válido con rol no admitido por la ruta recibe `403`.
- Un servicio rechaza con `403` el acceso a un recurso ajeno aunque el rol general sea válido.
- Una asignación revocada impide la siguiente operación protegida y una falla de members produce `503` sin modificar datos.
- Una API key inválida, revocada o de un partner inactivo recibe `401`; una cuenta no vinculada se oculta con `404` según el contrato.
- En la configuración de producción solo Traefik publica puertos; el gateway, los servicios y los almacenes no son alcanzables directamente desde Internet.
- Logs y respuestas públicas no contienen credenciales ni datos personales.
