# ADR-011 v2 — Primera observabilidad del flujo de partners

| Campo | Valor |
|---|---|
| Estado | Aceptado; primera traza y logs implementados en etapa 4, métricas y alertas pendientes |
| Fecha | 2026-10-16 |
| Decisión del TP | D11 |
| Reemplaza | [ADR-011](ADR-011-observabilidad-v1.md) |
| Registro | D-48 y D-55 |

## Contexto y alternativas

Se mantiene el diseño inicial aprobado: logs JSON con correlación y trazas Gateway → Benefits → PostgreSQL mediante OpenTelemetry, Collector y Jaeger. Solo logs no permiten demostrar los spans distribuidos exigidos por Entrega 2; desplegar toda la suite agrega alcance innecesario.

Para el historial local se consideraron almacenamiento persistente y memoria con exportación de evidencia. El equipo aprobó memoria para evitar otra base durante esta demostración. No es una política de retención productiva.

## Decisión

Instrumentar HTTP entrante y saliente del flujo mínimo y agregar spans manuales en el repositorio PostgreSQL (transacción completa, listado y autenticación). Propagar W3C traceparent. No registrar SQL, parámetros, DSN, cuerpos, headers, identificadores de usuario/cuenta/partner ni claves de idempotencia. Los logs incorporan trace_id y span_id del contexto activo y conservan correlation_id. El hecho de acreditación registra únicamente el resultado y si fue replay.

Se conservan muestreo local del 100 % y presupuestos funcionales ya aprobados. Collector envía OTLP a Jaeger por la red interna de Compose; solo la interfaz de consulta se publica en loopback. Jaeger usa memoria y pierde historial al reiniciar; se exporta una traza y logs sanitizados al repositorio (D-55). Se usan configuración mínima y versiones fijadas, sin afinación de los valores de batching del fabricante. El límite de memoria en cantidad de trazas replica el ejemplo oficial de Jaeger y no representa capacidad medida.

Un fallo de exportación se informa sin registrar errores crudos; no cambia la transacción de negocio ni se agrega Collector a readiness crítica. La entrega de trazas es de mejor esfuerzo: memoria y colas no garantizan conservación ante caída. El cierre intenta vaciar el exportador dentro del SHUTDOWN_TIMEOUT ya existente.

## Alcance y consecuencias

La primera versión cubre acreditación, saldo y movimientos. No incluye instrumentación de los otros servicios, métricas exportadas, alertas activas, Prometheus, Loki o Grafana. Permanecen los objetivos iniciales de métricas y alertas del ADR anterior, pendientes de implementación y validación; no se afirma que existan paneles ni SLA.

Positivo: permite relacionar logs con los tiempos y padres de cada span sin otra base de observabilidad. Límites: consumo adicional, historial volátil y spans de repositorio agregados (no de cada sentencia SQL). El exportador usa colas acotadas del SDK; puede descartar trazas si Collector no recibe a tiempo.

## Fundamento y fuentes

Teórico: correlación y trazabilidad, pp. 7–8; señales de resiliencia, pp. 111–112; separación mediante puertos/adaptadores, pp. 120–124. El PDF no enseña OpenTelemetry, Collector ni Jaeger: son herramientas adicionales aprobadas para producir la traza exigida, no contenido atribuido a clase.

Configuración práctica consultada: [instrumentación Go](https://opentelemetry.io/docs/languages/go/instrumentation/), [configuración de Collector](https://opentelemetry.io/docs/collector/configuration/), [Jaeger local en memoria](https://www.jaegertracing.io/docs/2.22/getting-started/) y [ejemplo oficial 2.22](https://github.com/jaegertracing/jaeger/blob/v2.22.0/cmd/jaeger/config.yaml). SDK Go 1.34 compatible con Go 1.22; versiones fijadas para reproducibilidad, sin afirmar que sean las últimas.

## Verificación

Prueba automatizada de traceparent, relación padre-hijo y sanitización; acreditación real mediante Gateway; exportación de traza y logs vinculados. Evidencia exportada y comandos en [ETAPA_4_OBSERVABILIDAD.md](../entrega-2/ETAPA_4_OBSERVABILIDAD.md). Tests, vet y lint de los siete módulos correctos; E2E por gateway y sanitización de evidencia correctos.
