# Etapa 14 — Observabilidad: métricas, tablero y alertas

> Fecha: 11/11/2026. Alcance grupal mínimo de la etapa 14 del
> [plan](PLAN_ENTREGA_FINAL.md): poder ver el recorrido y una falla en un tablero sin
> inspeccionar cada componente a mano. Decisión D-93. Base: D-48 y
> [ADR-011 v2](../adr/ADR-011-v2-observabilidad-local.md).

## 1. Qué se agregó

| Pieza | Dónde | Qué hace |
|---|---|---|
| Prometheus v2.55.1 | `deploy/prometheus/prometheus.yml` · `127.0.0.1:9090` | Lee cada 5 a 10 s el `/metrics` del gateway, Benefits, Booking, el indexer, notification-worker y RabbitMQ. Retención de 2 días. |
| Alertas | `deploy/prometheus/alertas.yml` | `ServicioCaido`, `MensajesEnDLQ` y `MetricasNoDisponibles`. Las evalúa Prometheus; se ven en `/alerts` y en el tablero. No hay Alertmanager: en local solo se muestran. |
| Grafana 11.3.0 | `deploy/grafana/` · `127.0.0.1:3000` | La fuente de datos y el tablero se cargan solos al arrancar. El tablero es la página de inicio y se ve sin iniciar sesión (rol Viewer). El administrador usa `GRAFANA_ADMIN_PASSWORD` del `.env`. |
| Métricas del gateway | `gateway/internal/server/metrics.go` | `gateway_http_requests_total` y `gateway_http_request_seconds_sum` por clase de respuesta (tasa, errores y duración del método RED). `gateway_upstream_up`, `_degraded` y `_latency_seconds` por servicio, con el mismo chequeo de `/api/v1/status`. `/metrics` responde siempre, con o sin limitador. |
| Métricas de MELT | `services/benefits-service/internal/application/melt.go` | `melt_envios_intentos_total{resultado}`: acreditado, pendiente_registro, rechazado o reintento. |
| Colas | plugin `rabbitmq_prometheus` (viene en la imagen) | `rabbitmq_detailed_queue_messages_ready{queue}` por cola, incluidas las de reintento y las DLQ. |

Las métricas no llevan ruta, usuario, email ni API key como etiqueta, para no crear
series sin límite ni exponer datos personales (CLAUDE.md, Logs).

## 2. Tablero «Metal Fitness — recorrido y salud»

1. **Estado del sistema:** cada servicio en OK o CAÍDO, cantidad de alertas activas y
   mensajes en DLQ.
2. **Entrada (api-gateway):** requests por segundo por clase (2xx/4xx/5xx), duración
   media y latencia del chequeo de salud de cada servicio.
3. **Control de tráfico:** rechazos `429` por política (login, user, partner),
   permitidas y errores de Redis.
4. **Eventos:** eventos procesados por consumidor, reintentos y DLQ acumulados, y
   mensajes esperando por cola.
5. **Caché, búsqueda y MELT:** aciertos y fallos de la caché del catálogo, retraso
   del índice de clases y envíos a MELT por resultado.

El tablero enlaza a las alertas de Prometheus, a Jaeger (trazas) y a RabbitMQ.

## 3. Validación del 11/11/2026 sobre el stack local completo

| Prueba | Resultado |
|---|---|
| Los 6 objetivos de Prometheus | `up` |
| Tráfico al gateway: 120 requests y 7 logins fallidos | Panel de requests con 2xx y 4xx. El sexto y el séptimo login dieron `429` y aparecen en «Rechazos 429 por política» (login). |
| **Falla controlada:** `docker stop gym-training-service-1` a las 14:26:13 | A los 45 s, `ServicioCaido{service="training-service"}` estaba en *firing*: el tablero mostró «CAÍDO» en rojo y «Alertas activas = 1». |
| Recuperación: `docker start` | El servicio volvió a *healthy* y la alerta se resolvió sola, sin alertas activas. |
| MELT | `melt_envios_intentos_total{resultado="pendiente_registro"}` sube con la reconsulta de un envío pendiente de registro (cada 2 min). |
| Tests | `go vet` de gateway y benefits; tests de `gateway/internal/server` (incluye `TestMetrics_RequestsYEstadoDeServicios`) y de application, domain y adaptador MELT de Benefits. |

## 4. Cómo se usa en la demostración

```sh
make demo                       # o make up con el .env existente
# Tablero:  http://localhost:3000   (se abre directo, sin login)
# Alertas:  http://localhost:9090/alerts
# Trazas:   http://localhost:16686
```

Para mostrar una falla: `docker stop gym-training-service-1`. A los ~20 s el tablero
pasa a CAÍDO y la alerta se dispara. Después, `docker start gym-training-service-1`.

## 5. Qué queda fuera (defensa individual o deuda)

- **Trazas:** siguen solo en gateway y Benefits (etapa 4 de Entrega 2). Members,
  Booking y el paso por RabbitMQ no propagan `traceparent`.
- **Logs:** sin Loki. Los logs JSON con `correlation_id` se consultan con
  `make logs s=<servicio>`.
- **Métricas de Members y training:** no exponen `/metrics` propios. Su
  disponibilidad se mide desde el gateway con `gateway_upstream_up`.
- **Alertas:** sin Alertmanager ni notificaciones salientes.
- **Nube:** el tablero es local, porque Vercel no corre Prometheus (L-19).
- **Postmortem:** POSTMORTEM.md sigue siendo el ejercicio de Entrega 2. El ejercicio
  final con fallas de Redis, RabbitMQ y OpenSearch queda para la defensa individual.
- **Gateway:** `/metrics` del gateway es público en local. Expone el mismo estado que
  `/api/v1/status` y contadores agregados, sin datos personales.
