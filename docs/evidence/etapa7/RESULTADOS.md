# Resultados de carga de Benefits

Generado desde los JSON de cada fase; los percentiles no se promedian entre fases.
Tiempos HTTP de k6 en ms; throughput en solicitudes/s. Calentamiento excluido de esta tabla.

| Escenario | VU | req/s | Operación | p50 ms | p95 ms | p99 ms | Umbral |
|---|---:|---:|---|---:|---:|---:|---|
| read-hot | 1 | 9.14 | read_ms | 5.73 | 18.06 | 22.06 | Cumple |
| read-hot | 5 | 45.35 | read_ms | 6.18 | 22.80 | 48.37 | Cumple |
| read-hot | 10 | 89.45 | read_ms | 6.43 | 27.78 | 49.22 | Cumple |
| read-distributed | 1 | 9.04 | read_ms | 6.30 | 18.64 | 29.16 | Cumple |
| read-distributed | 5 | 44.34 | read_ms | 6.33 | 32.96 | 76.27 | Cumple |
| read-distributed | 10 | 84.70 | read_ms | 7.32 | 56.65 | 140.27 | Cumple |
| write-hot | 1 | 8.21 | write_ms | 11.20 | 63.61 | 96.19 | Cumple |
| write-hot | 5 | 39.17 | write_ms | 13.44 | 75.49 | 169.88 | Cumple |
| write-hot | 10 | 73.04 | write_ms | 14.91 | 110.51 | 352.47 | Cumple |
| write-distributed | 1 | 7.82 | write_ms | 11.76 | 87.86 | 159.39 | Cumple |
| write-distributed | 5 | 32.85 | write_ms | 30.24 | 146.10 | 320.28 | Cumple |
| write-distributed | 10 | 74.92 | write_ms | 16.28 | 95.99 | 244.69 | Cumple |
| replay-hot | 1 | 7.97 | replay_ms | 11.58 | 71.53 | 151.84 | Cumple |
| replay-hot | 5 | 35.70 | replay_ms | 9.23 | 239.12 | 393.17 | Cumple |
| replay-hot | 10 | 78.54 | replay_ms | 8.21 | 100.05 | 294.02 | Cumple |
| replay-distributed | 1 | 8.28 | replay_ms | 8.11 | 59.87 | 132.18 | Cumple |
| replay-distributed | 5 | 43.24 | replay_ms | 7.36 | 38.32 | 124.10 | Cumple |
| replay-distributed | 10 | 82.23 | replay_ms | 8.78 | 67.30 | 192.60 | Cumple |
| mixed-hot | 1 | 8.76 | read_ms | 5.61 | 29.62 | 108.73 | Cumple |
| mixed-hot | 1 | 8.76 | write_ms | 9.39 | 33.82 | 161.65 | Cumple |
| mixed-hot | 5 | 43.33 | read_ms | 7.96 | 31.65 | 70.92 | Cumple |
| mixed-hot | 5 | 43.33 | write_ms | 11.41 | 36.36 | 106.20 | Cumple |
| mixed-hot | 10 | 84.10 | read_ms | 9.68 | 41.75 | 112.72 | Cumple |
| mixed-hot | 10 | 84.10 | write_ms | 12.38 | 47.78 | 120.89 | Cumple |
| mixed-distributed | 1 | 8.33 | read_ms | 8.16 | 67.74 | 117.97 | Cumple |
| mixed-distributed | 1 | 8.33 | write_ms | 12.73 | 66.98 | 106.27 | Cumple |
| mixed-distributed | 5 | 40.30 | read_ms | 11.11 | 68.47 | 113.58 | Cumple |
| mixed-distributed | 5 | 40.30 | write_ms | 15.98 | 77.35 | 121.19 | Cumple |
| mixed-distributed | 10 | 74.60 | read_ms | 13.98 | 90.82 | 219.80 | Cumple |
| mixed-distributed | 10 | 74.60 | write_ms | 19.74 | 121.63 | 291.23 | Cumple |

## Estado final por escenario

Incluye precarga y calentamiento. Las lecturas y los replays no deben crear movimientos.

| Escenario | Cuentas | Suma saldos | Movimientos | Operaciones | Cuentas inconsistentes |
|---|---:|---:|---:|---:|---:|
| read-hot | 100 | 1000 | 1000 | 1000 | 0 |
| read-distributed | 100 | 1000 | 1000 | 1000 | 0 |
| write-hot | 100 | 8487 | 8487 | 8487 | 0 |
| write-distributed | 100 | 8165 | 8165 | 8165 | 0 |
| replay-hot | 100 | 1000 | 1000 | 1000 | 0 |
| replay-distributed | 100 | 1000 | 1000 | 1000 | 0 |
| mixed-hot | 100 | 2691 | 2691 | 2691 | 0 |
| mixed-distributed | 100 | 2537 | 2537 | 2537 | 0 |

## Máximos de las muestras de recursos

Muestreo objetivo cada 5 s más el tiempo de consulta; no representa máximos instantáneos ni prueba ausencia de esperas breves.
CPU Docker: 100 % equivale aproximadamente a un núcleo; valores mayores son posibles.

| Servicio | CPU % | Memoria MiB |
|---|---:|---:|
| benefits | 69.89 | 17.63 |
| collector | 21.29 | 44.09 |
| gateway | 65.17 | 14.87 |
| generator | 58.81 | 20.01 |
| jaeger | 211.46 | 302.90 |
| postgres | 95.77 | 132.40 |
| Otros contenedores, agregados | 446.93 | 1119.69 |

PostgreSQL: hasta 8 sesiones en benefits_db (incluye procesos internos y otras conexiones; excluye la consulta de monitoreo); 8 activas; 7 esperando locks y 1 esperando I/O en las muestras (máximos independientes).

## Plan del agregado del ledger, cuenta 0

Consulta diagnóstica posterior a cada combinación; no mide la transacción completa ni modifica el esquema.

| Escenario | Acceso elegido | Filas devueltas al agregado | Ejecución ms |
|---|---|---:|---:|
| read-hot | Bitmap Heap Scan | 10 | 0.469 |
| read-distributed | Bitmap Heap Scan | 10 | 0.369 |
| write-hot | Seq Scan | 7497 | 2.632 |
| write-distributed | Bitmap Heap Scan | 99 | 1.004 |
| replay-hot | Bitmap Heap Scan | 10 | 0.347 |
| replay-distributed | Bitmap Heap Scan | 10 | 0.452 |
| mixed-hot | Bitmap Heap Scan | 1701 | 0.728 |
| mixed-distributed | Bitmap Heap Scan | 28 | 0.328 |

Fases pendientes: 0. Fases con falla funcional o inconsistencia: 0.
Objetivos de latencia incumplidos en niveles medidos (sin calentamiento): 0.
