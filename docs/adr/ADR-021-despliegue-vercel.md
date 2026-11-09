# ADR-021 — Publicar la capacidad compartida en Vercel con PostgreSQL gestionado

| Campo | Valor |
|---|---|
| **Estado** | Aceptado por el equipo el 09/11/2026 (D-75) |
| **Fecha** | 2026-11-09 |
| **Decisión del TP** | Adicional (distribución en la nube); impacta la estimación de costos de D13 |
| **Reemplaza** | D-41 (Oracle Cloud Free Tier) y la decisión objetivo de [ARCHITECTURE §14.2](../ARCHITECTURE.md#142-nube) |
| **Relacionado con** | D-34, D-69, D-75; [ADR-006](ADR-006-seguridad.md); [ADR-008 v4](ADR-008-v4-capacidad-operativa-club.md); [ADR-013 v2](ADR-013-v2-capacidad-costos-medidos.md); [ADR-022](ADR-022-registro-externo-members-benefits.md); [guía de despliegue](../deploy/VERCEL_CLUB.md) |

## Contexto

El enunciado exige que la capacidad propia esté desplegada en la nube y accesible
mediante una URL pública, y que se mantenga operativa hasta el final de la
evaluación (§4.1 y §6). D-41 había elegido una VM ARM de Oracle Cloud Free Tier con
Docker Compose, pero esa VM nunca se creó. El 08/11/2026 el equipo publicó el Club en
Vercel desde Vercel CLI y verificó registro, crédito, saldo, movimientos e
idempotencia contra producción. Esta decisión registra ese cambio, que no estaba
documentado y contradecía D-41 y D-69.

## Alternativas consideradas

| Alternativa | Ventaja | Desventaja |
|---|---|---|
| VM de Oracle Cloud con Docker Compose (D-41) | Paridad total con el entorno local, incluidos RabbitMQ, Redis y observabilidad | Requiere crear y operar la VM, TLS y firewall; la cuenta y la VM no existían a la fecha |
| Vercel Services + PostgreSQL gestionado (elegida) | URL HTTPS disponible sin operar servidores; despliegue desde la CLI sin conectar el repositorio | Funciones beta; sin broker, caché ni stack de observabilidad propios en la nube |
| No publicar hasta tener la VM | Sin cambios de plataforma | Incumple la capacidad accesible para el grupo consumidor |

## Decisión

Publicar en un proyecto de Vercel los servicios necesarios para la capacidad
compartida y el acceso del usuario, definidos en [vercel.json](../../vercel.json):

| Componente en Vercel | Rol |
|---|---|
| `gateway` (contenedor) | Única entrada de `/partner-api/*` y `/api/*` |
| `benefits` (contenedor) | Ledger, API de partners y registro externo |
| `members` (contenedor) | Identidad, acceso y membresías |
| `members-registration` (contenedor) | Segunda instancia de Members sin ruta pública, que recibe los registros desde Benefits (ver ADR-022) |
| `members-worker` (Daemon) | Completa registros, envía correos y vence membresías |
| `web` (Vite) | Frontend estático, mismo dominio |

Las bases `members` y `benefits` son lógicas e independientes en un PostgreSQL
gestionado con TLS (Neon, vía el Marketplace de Vercel). Cada servicio sigue
accediendo solo a su base (regla 3). El correo usa un proveedor SMTP con STARTTLS.
Los secretos se cargan como variables secretas de Vercel; nunca en Git ni en el
frontend. Docker Compose sigue siendo el entorno local y el de pruebas.

URL pública: `https://metalfitness-club.vercel.app`. Base para partners:
`https://metalfitness-club.vercel.app/partner-api/v2`.

## Consecuencias

### Positivas

- La capacidad compartida es accesible por HTTPS sin depender de una computadora
  del equipo encendida.
- El repositorio no se conecta a Vercel: publicar exige un `vercel deploy --prod`
  explícito, lo que evita despliegues accidentales desde `main`.

### Negativas y límites aceptados

- **Diferencias con el entorno local.** En la nube no hay RabbitMQ, Redis,
  OpenSearch, Collector ni Jaeger. Las trazas y el rate limiting general del
  gateway no existen en producción; los límites por integración viven en
  PostgreSQL (ADR-016). Ver ARCHITECTURE §15.
- **Plataforma beta.** Vercel Services y Daemons están en beta; el Daemon solo corre
  en Production y consume cuota de memoria del plan.
- **Ciclo de servicios.** Vercel no admite referencias circulares entre servicios,
  por eso existe `members-registration` (ADR-022).
- **Costos.** La estimación de D13 (ADR-013 v2) corresponde a Oracle en São Paulo y
  ya no describe lo desplegado. El costo y las cuotas del plan usado deben
  verificarse en el panel de Vercel y del proveedor PostgreSQL; este ADR no los
  estima ni mide capacidad en la nube.
- **Versión publicada.** Sin integración con Git, la versión en producción puede no
  coincidir con `main`. Antes de una demostración se debe publicar desde la rama
  aprobada y registrar el commit desplegado.

## Fundamento

Capacidad, latencia y costos (pp. 29–30 y 143 del teórico, según
[BASE_TEORICA.md](../BASE_TEORICA.md)). Vercel, sus Services/Daemons y Neon son
herramientas adicionales elegidas por el equipo para cumplir la publicación exigida
por el TP; no se atribuyen al apunte.

## Cómo se verifica

- Una solicitud sin API key a `/partner-api/v2/accounts/{id}/balance` responde 401 en
  formato de problema (verificado el 09/11/2026).
- El recorrido de `scripts/smoke-club-vercel.ps1` y `scripts/check-club-credit-vercel.ps1`
  (registro, recibo, activación, crédito, saldo, movimientos e idempotencia), que el
  equipo completó en producción el 08/11/2026.
