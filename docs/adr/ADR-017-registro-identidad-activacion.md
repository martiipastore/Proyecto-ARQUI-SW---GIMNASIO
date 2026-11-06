# ADR-017 — Registro externo, identidad y activación

| Campo | Valor |
|---|---|
| Estado | Propuesto para revisión del equipo; CB-03 implementada y validada localmente con JWT conforme al ADR-006; sin despliegue |
| Fecha | 2026-11-06 |
| Autores | Borrador preparado por el asistente para el equipo METALFITNESS; revisión de otro integrante pendiente |
| Decisiones del TP | D1, D3, D5, D7, D8; aplica la seguridad aceptada del ADR-006 |
| Relacionado con | D-63/65/70; HU-CB-01/02; RN-CB-03/07/11–13; ADR-001/003/005 v2/006/007/018 |
| Etapa | CB-03; requisitos de registro autorizados, cambios técnicos no ratificados por implicación |

Este ADR **complementa, no reemplaza ADR-006**. D-70 detectó en el borrador sesiones
opacas de ocho horas y bcrypt costo 10. CB-03 corrigió esa divergencia implementando
JWT RS256 de 15 minutos, roles persistidos y costo mínimo 12. Las pruebas locales
están registradas; la revisión del equipo sigue pendiente. Los flags permanecen
apagados por defecto y no se alteró el ADR-006 aceptado.

## Contexto

El alcance revisado requiere usuarios sin membresía, registro externo con puntos
y acceso seguro sin contraseñas enviadas por correo. Members tenía solamente
infraestructura. No existían usuarios, login ni el emisor JWT contemplado en el diseño.

## Decisión

Mantener usuarios, contraseñas, identidad externa y comandos de registro
en members_db. Benefits mantiene sus propias cuentas, ledger, autorizaciones y
eventos. La relación es por UUID del usuario, no por membresía ni por un join a otra
base. Una cuenta nueva lleva origen externo; una existente conserva su origen.

El correo de la integración no acredita control de una cuenta existente. Members
exige confirmación de correo antes de declarar verificado un vínculo nuevo para
esa persona. Una identidad vinculada no se reasigna a otro usuario.

Members persiste el comando antes de responder 202 y lo procesa con reintentos.
Benefits valida permisos y acredita usando evento único e Idempotency-Key estable.
Members registra la finalización y su outbox de correo en una sola transacción.
No se implementa una transacción distribuida ni publicación directa a RabbitMQ.
Las colas PostgreSQL usan `FOR UPDATE SKIP LOCKED`, admitiendo varias réplicas.

Tokens de activación de 256 bits, hash verificable, copia cifrada AES-GCM para
outbox, expiración y consumo atómico. Solo se establece una contraseña si no había
una anterior. bcrypt se reutiliza desde la dependencia existente x/crypto.

Members emite JWT RS256 con sub UUID, role, iss=gym-members, aud=gym-api, iat y
exp, con vida de 900 segundos. El gateway verifica localmente firma y claims con
la clave pública; no consulta sesiones ni recibe la clave privada. Bcrypt usa
costo 12 para activación y actualiza hashes de costo menor tras un login válido,
mediante compare-and-set. Las cuentas externas nuevas tienen rol ALUMNO; el
registro no acepta privilegios y conserva los roles de cuentas previas.

El perfil acotado de JWT se implementa en pkg/access con encoding/json,
crypto/rsa, crypto/sha256 y crypto/x509 estándar de Go, sin dependencias nuevas.
Solo RS256, RSA >=2048 bits y claves PEM fijadas por configuración; no se negocian
algoritmos ni se consultan URLs de claves del token. El perfil admite aud como
string y rechaza claims/cabeceras ajenos al perfil; no es un procesador JOSE general.
Fuentes complementarias: RFC 7519, RFC 7518 §3.3 y documentación oficial crypto/rsa.

Cuenta propia y logout admiten los cuatro roles válidos. El resto de recursos
privados aún sin lógica de dominio quedan limitados a ADMINISTRADOR hasta definir
su matriz por operación en su etapa. Esa restricción conservadora no implementa
la administración ni sustituye autorización fina. Las cabeceras de identidad se
eliminan antes de inyectar sub/role verificados; internal permanece bloqueado.

Logout elimina el JWT de memoria en el cliente, incluso si falla la red. Según
ADR-006 v1 no hay refresh ni lista de revocación: una copia del JWT sigue vigente
hasta exp. El frontend descarta acceso al vencer o recibir 401. Se conserva la
tabla de sesiones del borrador para evitar borrados; sus tokens ya no autentican.

Las llamadas internas llevan credencial de servicio independiente de las API keys
de partners. Cifrado de activación usa una segunda clave para permitir rotar la
credencial interna sin inutilizar correos pendientes. Ningún secreto va a VITE.

## Alternativas consideradas

- Contraseña temporal por correo: descartada por exposición del secreto y por
  el requisito explícito de preferir activación segura.
- Vincular por correo afirmado por el partner: descartada; no prueba control de
  una cuenta existente y puede conceder acceso a información ajena.
- Comando persistente, prueba de correo y token de un solo uso: elegida localmente
  para cumplir identidad sin cambiar de base ni duplicar cuentas.
- Sesiones opacas: implementación del borrador descartada en CB-03 por divergir
  de ADR-006; la alternativa JWT aceptada ya se implementó y probó localmente.

## Fundamento y elecciones adicionales

BASE_TEORICA.md relaciona propiedad del dato/transacciones con pp. 20–30,
idempotencia/recuperación con pp. 103–105 y 113–115, y puertos/capas con
pp. 118 y 120–124. Tokens de 256 bits, AES-GCM, bcrypt, cuotas y tiempos concretos
son mecanismos/parámetros del proyecto, no contenidos acreditados al PDF.
Reutilizar bibliotecas no implica aprobación de esos detalles técnicos.

## Consecuencias y criterios de cierre

La divergencia de D-70 está corregida localmente; revisar evidencia y configuración
antes de habilitar. Costo de contraseña, duración/mecanismo de acceso,
roles y rutas coinciden con el perfil aceptado. Verificar alta
nueva/previa, token vencido/repetido, preservación de contraseña existente,
reintento tras commit remoto, caída SMTP, login/logout y aislamiento de saldo por
gateway frente a cabeceras falsas. La validación CB-03 pasó 100 resultados de
tests/subtests y vet en pkg/gateway/members, nueve escenarios reales HTTP/SMTP
locales y build frontend. Ver CB03_VALIDACION.md e informes en evidence/club.
La evidencia no sustituye revisión de otro integrante ni prueba productiva.

Registro y saldo pueden permanecer brevemente en estados distintos: el comando
durable permite completar la operación tras una caída sin duplicar puntos. Si se
revoca el permiso o se agota el presupuesto, el comando sigue pendiente y reintenta.
SMTP ofrece entrega al menos una vez, no exactamente una; Message-ID es estable
y un enlace duplicado no permite una segunda activación.

No se incorporan nuevas herramientas externas. SMTP exige STARTTLS salvo una
excepción explícita para Mailpit local sin credenciales. El despliegue se protege
con flags apagados por defecto y secretos obligatorios al activarlos.

Guía y límites operativos: [CLUB_REGISTRO_EXTERNO.md](../CLUB_REGISTRO_EXTERNO.md).

## Riesgos a revisar

| Riesgo | Impacto | Mitigación / aceptación pendiente |
|---|---|---|
| D-70 corregida en local, revisión pendiente | Falta aprobación de integración del cambio y configuración de claves | Revisar CB03_VALIDACION.md; no autoaprobar ni habilitar por pasar tests |
| Pérdida/rotación de la clave de cifrado con correos pendientes | Enlaces pendientes no recuperables desde la cola | Procedimiento de rotación y recuperación, separado de la credencial interna |
| Caída después del efecto remoto o del envío SMTP | Reintentos y posible correo repetido | Evento único y comando durable probados con respuesta perdida tras commit real; SMTP detenido/recuperado probado; una pérdida posterior al envío puede repetir el correo y no se promete exactamente una entrega |
El antecedente sin Docker queda reemplazado por la ejecución local con PostgreSQL
y Mailpit reales. No se declara el objetivo integral ni el despliegue terminados.
