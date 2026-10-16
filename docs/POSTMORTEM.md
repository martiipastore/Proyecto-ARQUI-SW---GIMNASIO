# Postmortem inicial — Ejercicio controlado de resiliencia

Fecha: 16/10/2026. Entorno: Docker Desktop local, stack gym; ejercicio planificado, sin usuarios reales. Responsable de ejecución: desarrollo del equipo. Relación: D10, D11; RN-22/RN-27, CL-08/08b. No es un incidente productivo ni una medición de SLA.

## Qué ocurrió e impacto

Se detuvo Benefits, se pausó PostgreSQL y se intercaló un proxy temporal para demorar una solicitud antes de Benefits y perder una respuesta después de confirmar la transacción. Las acreditaciones sintéticas recibieron 502/503/504 según el punto de falla. Durante la pausa, la instancia compartida de PostgreSQL del gimnasio dejó de atender también a sus otros servicios SQL. Se preservaron los volúmenes y los fixtures son cuentas de prueba aisladas.

La falla de mayor interés fue una confirmación 201 que el partner no recibió: Gateway devolvió 504 aunque el cuarto movimiento ya estaba guardado. Reintentar la misma clave recuperó el cuerpo original exacto y no creó un quinto movimiento. No se confirmó pérdida de datos ni duplicación.

## Línea temporal de la corrida final

Horas de Argentina (UTC−3), derivadas de los timestamps UTC conservados en results.json. Incluyen preparación/recreación de contenedores; no representan solo duración de requests.

| Hora | Hecho |
|---|---|
| 18:34:00 | Se detiene Benefits |
| 18:34:11 | Benefits recupera disponibilidad |
| 18:34:12 | Se pausa PostgreSQL |
| 18:34:19 | PostgreSQL restaurado y Benefits listo |
| 18:34:19 | Se inicia demora previa a Benefits |
| 18:34:45 | Ruta directa restaurada |
| 18:34:45 | Se inicia pérdida de respuesta confirmada |
| 18:34:59 | Reintento recupera resultado sin duplicar |
| 18:35:07 | Stack normal restaurado; proxy retirado |

## Detección y diagnóstico

Estados HTTP contractuales, recuentos SQL antes/después, liveness/readiness, logs JSON y trazas con trace_id compartido. La evidencia relaciona catorce líneas de logs con cinco trazas; el inyector registra que consumió un 201 y solo el hash del cuerpo. El replay presenta el mismo hash. Los cuerpos, credenciales y IDs del fixture no se exportan.

| Escenario | Respuesta | Duración ms | trace_id |
|---|---|---|---|
| Benefits detenido | 502 UNAVAILABLE | 3370.831 | d541fae3a6ab4f43a6bfd81cc617fd1b |
| PostgreSQL pausado | 503 UNAVAILABLE | 4004.363 | 575f6a98041c46008397d5a1b9483682 |
| Espera antes de Benefits | 504 TIMEOUT | 5010.314 | 4026873a3e464cc6ae33f07387402448 |
| Respuesta perdida después de commit | 504 TIMEOUT | 5004.196 | 96d97f9483554be39e47e3bab7bc1414 |

## Causa e interpretación

Las fallas fueron inducidas deliberadamente. Un timeout informa que venció la espera; no determina si el commit ocurrió. La diferencia entre las dos pruebas 504 se comprobó en la base: antes de Benefits no había nuevo movimiento; después del commit ya existía. El resultado durable por partner/clave permitió resolver la incertidumbre.

La prueba de cancelación previa al commit con PostgreSQL real confirma rollback de movimiento, saldo y resultado idempotente. Esto no equivale a garantizar rollback ante toda pérdida de conexión durante commit.

## Recuperación y reconciliación

Se reactivó Benefits, se despausó PostgreSQL, se restauró la URL directa del Gateway y se retiró el proxy. Cada recuperación usó la misma clave original. Estado final: saldo 400, cuatro movimientos y cuatro operaciones; tres operaciones creadas tras restauración y una confirmada antes de perder la respuesta. Verificación final: trece contenedores healthy, Collector/Jaeger en ejecución y estado agregado ready.

## Hallazgos y acciones

| Hallazgo | Acción | Estado |
|---|---|---|
| El primer montaje del inyector falló porque el contexto raíz excluye tests mediante .dockerignore | Darle un contexto propio, sin ampliar los contextos normales; el finally restauró los servicios incluso en ese intento | Corregido y repetido |
| Recrear Gateway elimina los logs del contenedor anterior | Capturar logs de cada falla antes de recrearlo; exigir logs del Gateway para cada traza exportada | Corregido y validado |
| Respuesta perdida tras commit deja incertidumbre al cliente | Conservar misma clave y recuperar resultado durable; comparar cuerpo por hash y recuentos | Verificado |
| Medición incluye transporte y planificación local | Margen aprobado D-56 de 500 ms; presupuestos del código siguen en 4 s/5 s | Documentado |
| Pausar la instancia SQL impacta a los servicios del gimnasio que la comparten | Ejecutar sin uso concurrente, restauración en finally y comprobación de readiness | Aplicado; límite del ejercicio |

## Límites y seguimiento

El ejercicio no caracteriza carga sostenida, costo, todas las fallas de red ni la capacidad del proveedor externo. Esas conclusiones no se infieren de cuatro fallas sintéticas. Continúa etapa 7 para D13; la etapa 5/D9 espera proveedor. Métricas exportadas, alertas activas y retención productiva de trazas siguen pendientes según D11.

Evidencia: [resultados](evidence/etapa6/results.json), [trazas](evidence/etapa6/traces.json), [logs](evidence/etapa6/logs.jsonl). Reproducción y restauración manual en [ETAPA_6_RESILIENCIA.md](ETAPA_6_RESILIENCIA.md).
