# ADR-011 — Instrumentar el flujo de partners con logs y trazas correlacionadas

| Campo | Valor |
|---|---|
| **Estado** | Reemplazado por [ADR-011 v2](ADR-011-v2-observabilidad-local.md) el 16/10/2026; se conserva el diseño inicial |
| **Fecha** | 2026-10-16 |
| **Decisión del TP** | D11 |
| **Autores** | Equipo del TPI |
| **Relacionado con** | ADR-005 v2; ADR-010; ADR-013; ARCHITECTURE §11 |

## Contexto

La Entrega 2 exige logs estructurados y correlacionados y una primera traza distribuida. El esqueleto ya usa slog JSON y X-Correlation-ID; eso no prueba la existencia de spans distribuidos ni métricas de negocio.

## Alternativas consideradas

| Alternativa | Ventaja | Desventaja |
|---|---|---|
| Solo logs | Menor costo inicial | No muestra duraciones y relaciones de spans; insuficiente para la entrega |
| Instrumentar todo el stack ahora | Visibilidad amplia | Mayor consumo y alcance antes de tener negocio |
| Instrumentar primero Gateway → Benefits → PostgreSQL | Evidencia directa sobre el flujo mínimo | El resto del stack queda para etapas posteriores |

## Decisión

Aplicar la arquitectura objetivo existente al flujo mínimo: OpenTelemetry en HTTP entrante/saliente y persistencia, propagación W3C traceparent, OTLP a Collector y Jaeger para visualizar. Logs JSON a stdout con service, level, msg, correlation_id, trace_id y span_id. Un correlation_id vincula logs; un trace_id une spans con relaciones y tiempos.

Se aprueba muestreo del 100 % en la demostración local para que cada request tenga evidencia. El muestreo y la retención de producción quedan pendientes de capacidad y costos. Registrar nombres de operación y rutas plantilla; excluir DNI, emails, externalUserId, API keys, tokens, claves de idempotencia y cuerpos completos de logs, etiquetas y atributos.

Las métricas iniciales serán cantidad, duración y errores por servicio/operación, errores y duración de PostgreSQL, replay y conflicto de idempotencia. No usar identificadores de cuenta/partner como etiquetas. Su exportación y tablero se incorporarán según el avance; Prometheus, Loki y Grafana siguen siendo el objetivo general y no se consideran desplegados.

Objetivo inicial de servicio: lecturas p95 < 200 ms, escrituras p95 < 500 ms y cero duplicaciones en el escenario controlado de ADR-013. Umbrales iniciales de alerta aprobados para el entorno de demostración: readiness crítica en 503 durante 30 s o errores 5xx superiores al 1 % durante 5 min con al menos 100 solicitudes. Estos umbrales no constituyen un SLA validado.

## Consecuencias

- Positiva: se puede localizar la dependencia que consume el tiempo y relacionar una falla con sus logs.
- Negativa: Collector y Jaeger consumen recursos adicionales; el muestreo completo no es una política productiva definitiva.
- Riesgo: atributos sensibles o cardinalidad alta. Mitigación: lista acotada de campos y revisión de datos exportados.

## Cómo se verificará

Etapa 4: una acreditación por gateway produce una traza visible con spans de gateway, Benefits y PostgreSQL, un mismo trace_id y logs correlacionados. Guardar exportación/captura sanitizada y test de propagación; inspeccionar ausencia de secretos. Diferenciar spans realmente instrumentados de los pendientes.

## Fundamento de clase y elección práctica

El fundamento de clase es correlación y trazabilidad (pp. 7–8) y las señales de resiliencia (pp. 111–112). OpenTelemetry, Collector y Jaeger son herramientas adicionales para obtener la traza exigida por la Entrega 2; el PDF no enseña sus productos ni configuración. El equipo autorizó herramientas necesarias y útiles con esta justificación. Muestreo y alertas son parámetros propios aprobados, pendientes de validar. No se despliega toda la suite de observabilidad por anticipado.
