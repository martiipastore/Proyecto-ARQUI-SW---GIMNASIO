# ADR-013 — Medir el flujo de Benefits antes de decidir su escalado

| Campo | Valor |
|---|---|
| **Estado** | Reemplazado por [ADR-013 v2](ADR-013-v2-capacidad-costos-medidos.md) el 16/10/2026; se conserva el plan inicial como historia |
| **Fecha** | 2026-10-16 |
| **Decisión del TP** | D13 |
| **Autores** | Equipo del TPI |
| **Relacionado con** | RN-22, RN-26, RN-27; D-34 y D-41 del registro; ADR-010; ADR-011 |

## Contexto

El equipo eligió una VM ARM de Oracle Cloud Free Tier con Docker Compose (D-41). Esa elección no demuestra disponibilidad de recursos, suficiencia de memoria ni costo final. La etapa 7 debe medir capacidad y justificar una alternativa de mejora.

## Alternativas consideradas

| Alternativa | Ventaja | Desventaja |
|---|---|---|
| Escalar procesos por anticipado | Más capacidad potencial | Costos y complejidad sin identificar el límite |
| Medir solo endpoints sin datos reales | Ejecución simple | No observa locks, pool ni idempotencia |
| Carga contractual con PostgreSQL y datos controlados | Mide el flujo que se entrega | Requiere fixtures y ambiente descrito |

## Decisión

Medir con k6 Partner → Gateway → Benefits → PostgreSQL. Hipótesis: las escrituras sobre una cuenta contienden por su bloqueo; entre cuentas, el pool o PostgreSQL pueden dominar. Se validará, no se presenta como un cuello de botella ya observado.

Plan reproducible inicial aprobado: 1 partner activo, 100 cuentas vinculadas y datos sintéticos; escenarios separados para cuenta caliente y cuentas distribuidas. Calentamiento 30 s y escalones de 1, 5 y 10 usuarios virtuales durante 60 s cada uno. Separar lecturas, créditos únicos, replay de créditos y mezcla 80 % lecturas / 20 % escrituras. Medir saldo y movimientos paginados y contabilizar replays por separado.

Umbrales iniciales ya previstos en el plan: 0 % de errores funcionales inesperados, cero movimientos duplicados, lecturas p95 < 200 ms y escrituras p95 < 500 ms. Los rechazos intencionales deben distinguirse de fallas del sistema.

Registrar p50/p95/p99, throughput, errores, CPU, memoria, conexiones y esperas de PostgreSQL, configuración del pool, versiones, fecha, arquitectura, datos y procesos externos concurrentes. La línea base local tuvo Docker con aproximadamente 3,7 GiB de memoria y otros proyectos activos; no es evidencia de capacidad de producción.

Comparar al menos dos mejoras con el límite medido: optimizar índices/consultas/transacciones y dimensionar pool; aumentar recursos de VM/base; o replicar Benefits si limita CPU del proceso. Agregar réplicas no elimina un bloqueo sobre la misma cuenta.

## Capacidad y costos pendientes

| Componente | Información requerida para la estimación |
|---|---|
| VM ARM | Región, forma, OCPU, RAM, disponibilidad y elegibilidad verificadas |
| Almacenamiento | Volúmenes, tamaño, crecimiento de movimientos e idempotencia (≥ 30 días), backups |
| Red y acceso | Tráfico saliente, IP pública, dominio y TLS |
| Observabilidad | Memoria/disco y retención de trazas, logs y métricas |
| Comparación | Configuración elegida y alternativa paga, moneda, precio unitario, fecha y fuente oficial |

No se asigna costo cero al stack por el nombre Free Tier. La cotización se verificará al completar la etapa 7; no hay precios ni resultados medidos en este ADR.

## Consecuencias

- Positiva: el escalado se puede justificar con evidencia del flujo real.
- Negativa: resultados locales no se extrapolan directamente a la VM ARM.
- Riesgo: carga sin límites puede agotar el entorno. Mitigación: comenzar con escalones pequeños y registrar saturación antes de ampliarlos.

## Cómo se verificará

Etapa 7: scripts versionados, reporte de ambiente y resultados, invariantes comprobadas desde PostgreSQL, límite identificado, dos alternativas comparadas y estimación fechada con fuentes. Las cifras objetivo son hipótesis hasta ejecutar esas pruebas.

## Fundamento de clase y elección práctica

k6 y p95 no se explican en el PDF. Se proponen como instrumentos adicionales para ejecutar las pruebas de carga del TP y medir capacidad/latencia: k6 genera solicitudes controladas; p95 indica que el 95 % de las observaciones no supera ese tiempo. El equipo permite herramientas adicionales justificadas, y aprobó expresamente este plan D13 y sus objetivos iniciales el 16/10/2026. Ver BASE_TEORICA.md.

La autorización de k6 es específica para automatizar los escenarios de carga exigidos por el TP y guardar resultados repetibles. No amplía el alcance a otras herramientas: se prioriza lo visto en clase y cada excepción nueva debe justificarse y consultarse. Los objetivos p95 no son garantías de rendimiento ni valores prescritos por el teórico.
