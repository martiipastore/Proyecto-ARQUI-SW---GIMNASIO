# CB-03 — Registro, activación y acceso

Fecha: 06/11/2026. Implementación y validación local completadas; revisión del
equipo pendiente antes de integrar. Se corrigió la divergencia D-70 aplicando
ADR-006 sin editar su decisión aceptada. ADR-017 documenta la implementación y
continúa propuesto en los detalles sujetos a revisión. Sigue CB-04: membresías.

## Cambios

- JWT RS256 de 900 segundos: sub UUID, role, iss, aud, iat y exp. Gateway verifica
  firma, algoritmo exacto, emisor, audiencia, fechas y vida máxima con clave pública.
  Members conserva la privada. Claves RSA PEM >=2048 bits; errores de configuración
  no imprimen el material. Sin clave pública, las rutas privadas fallan cerradas.
  El JWT no se reenvía a los servicios: reciben solo actor/rol verificados y
  credencial interna. Members no vuelve a decodificar tokens del cliente.
- Roles persistidos por migración aditiva 002 de Members: ADMINISTRADOR, PROFESOR,
  NUTRICIONISTA y ALUMNO. El registro externo siempre comienza como ALUMNO y no
  acepta rol en su JSON. Las cuentas previas conservan su identidad y privilegios.
- Bcrypt de costo 12 para primera contraseña. Un login correcto actualiza hashes
  anteriores de costo menor mediante compare-and-set, sin restablecer contraseñas.
- Gateway limpia identidad del cliente y protege recursos privados. Cuenta propia
  y logout admiten roles válidos; los otros recursos aún por implementar quedan
  restringidos a administración hasta definir su matriz específica. El dominio
  sigue siendo responsable de propiedad y permisos finos.
- Se elimina el uso de sesiones opacas y la consulta /internal/v1/session. Su
  tabla se conserva sin uso para evitar borrar datos del borrador. JWT y roles no
  dependen de membresía. No se añadió un proveedor de identidad ni dependencias.
- Logout descarta el token del navegador; no revoca copias, conforme a ADR-006 v1.
  El frontend conserva el token solo en memoria, lo descarta al vencer o recibir
  401 y permite cerrar el acceso local incluso si hay un error de red.

## Evidencia

[Tests y vet](evidence/club/cb03-tests.json): 100 resultados de tests/subtests
aprobados y cero omitidos: pkg 53, gateway 25, Members 22. PostgreSQL real es
obligatorio bajo CI/REQUIRE_DOCKER_TESTS=1. Los paquetes sin tests no se cuentan
como tests omitidos. `go vet ./...` pasó en los tres módulos.

[E2E real local](evidence/club/cb03-e2e.json): nueve escenarios aprobados con
binarios reales de Members, Benefits y Gateway, PostgreSQL 16 y SMTP Mailpit
v1.20. Contenedores con nombres únicos y puertos loopback; datos y claves
sintéticos generados para esa ejecución, sin leer .env ni datos del gimnasio.

| Escenario | Resultado comprobado |
|---|---|
| Infraestructura y servicios reales | Bases separadas y roles de conexión propios; procesos listos |
| Alta nueva y respuesta perdida después del commit | Callback real acredita, proxy de pruebas corta respuesta; reintento completa recibo y bienvenida sin duplicar movimiento |
| Activación, login y cuenta sin membresía | Primera contraseña de un uso; JWT de 900 s; identidad externa ALUMNO y saldo propio de 150 |
| Frontera de confianza | Cabeceras falsas no suplantan; falta/vencimiento de JWT da 401; acceso administrativo con rol alumno da 403; internal da 404 |
| Eventos y aislamiento de partners | Registro/credits comparten evento; otra key no duplica; otra integración no consulta el saldo |
| Cuenta normal existente | Exige prueba de correo antes del vínculo; conserva UUID, origen normal y contraseña; hash costo 10 se actualiza a 12; saldo correcto |
| SMTP detenido y reiniciado | Comando/puntos completados y correo pendiente durable; reintento entrega bienvenida, sin duplicar crédito |
| Registro con cero puntos y enlace vencido | Cuenta/saldo cero; enlace vencido rechazado; reenvío rota token; anterior rechazado y nuevo se consume una vez |
| Logout v1 | Devuelve 204; una copia del JWT sigue válida hasta exp, según ADR-006 |

Además, tests unitarios verifican los cuatro roles, firma equivocada, algoritmos
none/HS256/RS512, issuer/audience/iat/exp/rol/sub inválidos, vida excesiva, PEM
inválido, claves débiles y ausencia de key. Build `npm.cmd run build` aprobado.
No se ejecutó automatización de navegador ni pruebas con el SMTP de producción.

## Reproducir

Desde la raíz, con Go y Docker disponibles:

```powershell
& ([scriptblock]::Create((Get-Content scripts/verify-club-access.ps1 -Raw -Encoding UTF8)))
go run ./tests/e2e/identity/main.go
```

El runner guarda resultados sin cuerpos, credenciales ni tokens. El E2E genera
su par RSA solo en memoria y crea recursos con prefijo gym-cb03 y nombres únicos.
Al terminar, detiene sus procesos y elimina únicamente sus contenedores; conserva
binarios/logs sintéticos en la carpeta temporal local. La primera ejecución puede
descargar las imágenes ya utilizadas por el proyecto y compilar binarios.
Para frontend: `npm.cmd run build` desde web.

## Configuración antes de habilitar

MEMBERS_JWT_PRIVATE_KEY y GATEWAY_JWT_PUBLIC_KEY deben formar el mismo par. Se
admite PEM multilínea o con secuencias literales \n. Mantener la privada fuera de
Git, frontend y logs; el gateway solo recibe la pública. No generar claves nuevas
en cada arranque: conservarlas en el gestor de secretos del entorno. Rotación v1
requiere coordinación y nueva autenticación; no hay JWKS ni múltiples keys/kid.

Aplicar migración 002 y coordinar el corte: las sesiones opacas anteriores ya no
son válidas y se requiere nuevo login. Los flags siguen apagados por defecto;
no se modificaron .env, claves reales, producción, Vercel o permisos de terceros.
Las cuotas iniciales siguen siendo parámetros operativos propuestos, no contratos
comerciales ratificados para plataformas reales.

La evidencia cubre CB-03 local. Membresías, recompensas, canjes, administración,
matrices de permisos de esos recursos, auditoría completa y los diez escenarios
integrales del Club siguen sus etapas. SMTP puede entregar un correo repetido si
se pierde su confirmación; el token de un uso y el evento durable limitan sus efectos.

## Fundamento y fuentes

Teórico: propiedad y transacciones, pp. 23/27–28; idempotencia y recuperación,
pp. 103–105/110/113–116; capas y dependencias, pp. 118/120–124. El diseño JWT,
bcrypt y sus valores provienen del ADR-006, no se atribuyen al apunte.
El perfil usa el estándar de [JWT](https://www.rfc-editor.org/rfc/rfc7519),
[RS256](https://www.rfc-editor.org/rfc/rfc7518#section-3.3) y las primitivas de
[crypto/rsa de Go](https://pkg.go.dev/crypto/rsa). El SMTP de pruebas usa la
[API de Mailpit](https://mailpit.axllent.org/docs/api-v1/) para comprobar recepción.
