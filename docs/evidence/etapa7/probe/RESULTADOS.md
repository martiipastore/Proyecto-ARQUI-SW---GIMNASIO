# Resultados de carga de Benefits

Generado desde los JSON de cada fase; los percentiles no se promedian entre fases.
Tiempos HTTP de k6 en ms; throughput en solicitudes/s. Calentamiento excluido de esta tabla.

| Escenario | VU | req/s | Operación | p50 ms | p95 ms | p99 ms | Umbral |
|---|---:|---:|---|---:|---:|---:|---|
| write-hot | 1 | 61.36 | write_ms | 12.17 | 36.45 | 64.01 | Cumple |
| write-hot | 5 | 90.27 | write_ms | 29.26 | 175.49 | 360.46 | Cumple |
| write-hot | 10 | 82.46 | write_ms | 57.38 | 472.24 | 1082.55 | Cumple |

## Estado final por escenario

Incluye precarga y calentamiento. Las lecturas y los replays no deben crear movimientos.

| Escenario | Cuentas | Suma saldos | Movimientos | Operaciones | Cuentas inconsistentes |
|---|---:|---:|---:|---:|---:|
| write-hot | 100 | 10954 | 10954 | 10954 | 0 |

## Máximos de las muestras de recursos

Muestreo objetivo cada 5 s más el tiempo de consulta; no representa máximos instantáneos ni prueba ausencia de esperas breves.
CPU Docker: 100 % equivale aproximadamente a un núcleo; valores mayores son posibles.

| Servicio | CPU % | Memoria MiB |
|---|---:|---:|
| benefits | 56.48 | 17.79 |
| collector | 11.59 | 43.46 |
| gateway | 34.59 | 12.77 |
| generator | 44.07 | 18.62 |
| jaeger | 5.74 | 341.80 |
| postgres | 181.43 | 161.50 |
| Otros contenedores, agregados | 269.47 | 1049.30 |

PostgreSQL: hasta 9 sesiones en benefits_db (incluye procesos internos y otras conexiones; excluye la consulta de monitoreo); 8 activas; 7 esperando locks y 1 esperando I/O en las muestras (máximos independientes).

## Plan del agregado del ledger, cuenta 0

Consulta diagnóstica posterior a cada combinación; no mide la transacción completa ni modifica el esquema.

| Escenario | Acceso elegido | Filas devueltas al agregado | Ejecución ms |
|---|---|---:|---:|
| write-hot | Bitmap Heap Scan | 9964 | 9.918 |

Fases pendientes: 0. Fases con falla funcional o inconsistencia: 0.
Objetivos de latencia incumplidos en niveles medidos (sin calentamiento): 0.
