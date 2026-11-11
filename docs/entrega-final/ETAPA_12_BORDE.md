# Etapa 12 — Entrada única, control de tráfico y balanceo

**Estado al 11/11/2026:**
- **Parte A, control de tráfico:** implementada y validada localmente.
- **Parte B, balanceo de Booking:** pendiente, planificada después de la presentación
  grupal.

El enunciado pide el «punto de entrada único con autenticación, direccionamiento y
**control de tráfico**» para la presentación grupal. El balanceo con evidencia de
distribución se evalúa en la defensa individual (§2.3 y «Defensa individual»).

Fuentes: [ARCHITECTURE §5.1 a §5.3](../ARCHITECTURE.md), D-79, D-91,
[ADR-007](../adr/ADR-007-cache-autorizacion-disponibilidad.md). Para la parte B: D-46,
D-51 y [ADR-012 v2](../adr/ADR-012-v2-estabilidad-health-checks.md).

## Parte A — Control de tráfico en el gateway

### Qué se agregó

Paquete `gateway/internal/ratelimit`: **token bucket atómico en Redis**. Un script Lua
recarga las fichas según el tiempo transcurrido y consume una en una sola operación.
Usa el reloj del servidor Redis, así que varias instancias del gateway comparten el
mismo contador sin carreras. Claves `gw:rl:<política>:<id>` con vencimiento automático.

| Política | Rutas | Clave | Límite | Si Redis no responde |
|---|---|---|---|---|
| `login` | `POST /api/v1/auth/login`, `activate` y `resend-activation` | IP de la conexión | 5 por minuto (ráfaga 5) | **Falla cerrado:** `503 DEPENDENCIA_NO_DISPONIBLE` con `Retry-After` |
| `user` | Resto de `/api/v1/**` autenticado | `sub` del JWT verificado | 10/s, ráfaga 20 | Falla abierto |
| `partner` | `/partner-api/v1` y `/v2` | SHA-256 de la API key (IP si falta) | 20/s, ráfaga 40 | Falla abierto |

Al excederse, la respuesta es `429` con `Retry-After` en segundos y problema RFC 7807:
`LIMITE_EXCEDIDO` en la API propia y `RATE_LIMITED` en el formato del contrato de
partners v2.

**Orden en el gateway (§5.1):** limpieza de identidad → autenticación → **límite** →
routing.
- El límite de usuario usa el `sub` verificado, nunca un `X-User-Id` enviado por el
  cliente.
- Un `401` se responde antes y no consume fichas.
- `/api/v1/status`, `/health/*` y `/metrics` no se limitan.
- La IP se toma de la conexión, no de `X-Forwarded-For`, que el cliente podría falsear
  para evadir el límite.

**Observabilidad.**
- `GET /metrics` del gateway expone `gateway_ratelimit_allowed_total{policy}`,
  `gateway_ratelimit_rejected_total{policy}` y `ratelimit_redis_errors_total`.
- Redis es una dependencia **parcial** de `/health/ready` (ADR-007): si cae, el gateway
  queda `degraded` sin salir de servicio.

**Configuración.**
- Prendido por defecto en Docker Compose (`GATEWAY_RATE_LIMIT_ENABLED=true`): Redis
  está siempre en el stack.
- Apagado si la variable no existe, como en Vercel, que no tiene Redis (D-79).
- Límites ajustables con `GATEWAY_RATE_LIMIT_*`.

### Pruebas

| Prueba | Resultado |
|---|---|
| Unitarias del gateway (`internal/server/ratelimit_test.go`, store en memoria) | Aprobadas: login 5 por IP con el sexto en 429 y `Retry-After`; usuario identificado por el JWT y no por `X-User-Id` falsificado; baldes independientes por usuario y por política; partners por hash de key con `RATE_LIMITED` y sin la key en claro; con Redis caído, login 503 y el resto pasa; status, health y 401 no consumen fichas; métricas |
| Integración con Redis 7 real (`internal/ratelimit`, testcontainers) | Aprobadas: ráfaga, rechazo con espera, recarga en el tiempo, 5/min = una ficha cada 12 s, claves independientes, vencimiento y **atomicidad con 100 solicitudes concurrentes** (exactamente 20 permitidas con ráfaga 20) |
| `go vet` y tests de config, proxy y server | Aprobados |

Los tests de `proxy` y `server` se ejecutaron en un contenedor `golang:1.22`: el control
de aplicaciones de Windows bloqueó los ejecutables de test en la carpeta temporal. El
test con Redis real corrió en Windows.

### Validación contra el stack real (11/11/2026)

| Escenario | Resultado |
|---|---|
| 6 logins seguidos desde la misma IP | 5 llegan a Members (401 por credenciales) y el sexto recibe **429 `LIMITE_EXCEDIDO`, `Retry-After: 11`** |
| Login después de esperar | 200 |
| 30 solicitudes simultáneas de un administrador | 23 aceptadas (ráfaga 20 más recarga) y **7 con 429** |
| 120 solicitudes de partner con la misma key en 1,6 s | 70 llegan a Benefits y **50 con 429** |
| Redis detenido | Login **503 `DEPENDENCIA_NO_DISPONIBLE`, `Retry-After: 1`**; un usuario autenticado recibe 200; `/health/ready` = `degraded`; `ratelimit_redis_errors_total` = 2 |
| Redis recuperado | Login 200 y `/health/ready` = `ready` sin reiniciar el gateway |

### Limitaciones

- **Todos los navegadores del Docker local comparten la IP de la red de Docker.** Las 5
  entradas por minuto del login se comparten entre todos los que prueban en la misma
  máquina. En una demostración con varios roles conviene no ingresar más de 5 veces por
  minuto, o subir `GATEWAY_RATE_LIMIT_LOGIN_PER_MINUTE` en `.env`.
- En la nube el límite general queda desactivado (D-79). El login sigue protegido por la
  cuota en PostgreSQL de Members y los partners por la cuota por integración de Benefits.

## Parte B — Balanceo de Booking (pendiente)

**Qué pide el enunciado (§2.3):**
- dos o más instancias de un servicio;
- reconocer una instancia no disponible;
- poder comprobar la distribución y el estado de las instancias con la observabilidad.

**No exige umbrales de fallos consecutivos.**

**Plan propuesto:**
- Dos instancias de `booking-service` detrás de Traefik, con igual peso y sin afinidad
  (D-46).
- Health check `GET /health/ready` cada 5 s con timeout de 2 s.
- Instancia identificada en logs y métricas para mostrar la distribución.
- Prueba de caída de una instancia.

Traefik retira una instancia al primer health check fallido; por lo que se sabe, no
tiene umbrales 3/2. Se confirmará con su documentación antes de implementar. Si se
confirma, la opción propuesta es registrar una decisión que reemplace D-51, aceptando la
mayor sensibilidad a demoras aisladas (flapping, teórico p. 87).
