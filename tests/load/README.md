# Carga contractual de Benefits (D13)

Ejecutar solo contra el stack local del gimnasio, con Docker Desktop iniciado, `.env` configurado y migraciones aplicadas según el README principal. El runner es para Windows PowerShell 5.1+ y usa la red `gym_default`. No requiere instalar k6 en Windows ni paquetes adicionales de Python.

Desde la raíz del repositorio:

```powershell
docker compose --env-file .env -f deploy/docker-compose.yml up -d --build --wait
docker pull grafana/k6:1.3.0
./tests/load/run-benefits.ps1
python tests/load/report.py
```

`python` debe identificar Python 3.10+; también puede usarse la ruta del intérprete instalado. El reporte se escribe en `docs/evidence/etapa7/RESULTADOS.md` y señala explícitamente las fases faltantes si la carga no terminó. Los JSON son la fuente de los resultados; no se suman ni promedian percentiles entre fases.

Una comprobación breve, fuera de la evidencia de capacidad, permite validar el entorno:

```powershell
./tests/load/run-benefits.ps1 -Smoke -Workloads read -Distributions hot -EvidenceDirectory "$env:TEMP/gym-load-smoke-evidence"
```

El modo breve conserva las 100 cuentas y 10 movimientos iniciales por cuenta; reduce calentamiento a 1 s y mide solo 1 VU durante 2 s. No sirve para evaluar los objetivos de capacidad. No se ejecuta automáticamente en CI ni sobre un entorno remoto.

Experimento adicional D-59, aprobado si la matriz inicial queda limitada por la pausa: ejecutarlo **después**, sin otras cargas simultáneas. Solo créditos sobre una cuenta, pausa 0, calentamiento 30 s y niveles 1/5/10 VU de 30 s; mismos datos iniciales y protecciones. Su evidencia queda separada:

```powershell
./tests/load/run-benefits.ps1 -Probe
python tests/load/report.py docs/evidence/etapa7/probe --probe
```

Al repetir pruebas, usar `-EvidenceDirectory` con una carpeta nueva para conservar la ejecución anterior. Sin ese parámetro, los informes del escenario elegido se sobrescriben en la carpeta predeterminada. La base siempre recibe fixtures nuevos.

## Qué se mide

- Cuatro cargas: lecturas alternadas de saldo/movimientos (primera página, tamaño 20), créditos únicos de 1 punto, replay exacto de créditos confirmados y mezcla 80 % lecturas / 20 % créditos.
- Cada carga se repite sobre una cuenta caliente y sobre 100 cuentas distribuidas: ocho combinaciones.
- Por combinación: nuevo partner con 100 cuentas y 10 movimientos sintéticos por cuenta, calentamiento de 30 s y niveles separados de 1, 5 y 10 VU durante 60 s cada uno. Los datos crecen entre niveles de esa combinación; no se reinician entre fases.
- Pausa aprobada D-57 de 100 ms después de cada operación. Las fases usan contenedores separados, sin superponer escalones. Se esperan las iteraciones en curso al terminar cada fase.
- Los 28 minutos son carga activa; preparar 8.000 movimientos por la API, arrancar contenedores y muestrear recursos aumenta la duración total.

Una operación de cada cinco en la mezcla es escritura, usando el contador global de iteraciones de la fase. En replay, cada VU usa su propia clave precargada por cuenta; compara estado HTTP, cabecera de replay y SHA-256 del cuerpo con la respuesta original. No se simulan replays con claves desconocidas.

El generador usa el prefijo público de partners del gateway desde la red Docker. La precarga pasa por el mismo contrato mediante `localhost:8080`. No se mide navegador, Internet, TLS ni latencia Córdoba–Brasil. Con pausa y 10 VU el techo teórico es 100 iteraciones/s incluso si el servicio respondiera instantáneamente; este experimento no mide por sí solo capacidad máxima.

## Integridad y evidencia

HU-29/HU-32, RN-22/RN-26/RN-27 y CL-08b: se comprueban respuestas y luego PostgreSQL tras cada fase. Debe haber 100 cuentas, cero diferencias por cuenta entre saldo y suma de movimientos, secuencia igual al número de movimientos y totales iguales a `1.000 + créditos nuevos confirmados` (incluye calentamiento). Replays y lecturas no agregan movimientos. Una falla funcional detiene la matriz; una falla solo de latencia conserva su resultado sin convertirlo en éxito.

`environment.json` conserva versión/recursos y configuración; cada fase contiene métricas k6, invariantes y muestras de CPU/memoria/esperas. Los p50/p95/p99 son de duración HTTP (envío, espera y recepción), excluyen la pausa y la adquisición inicial de conexión. `ledger-plan.json` conserva el plan real del agregado que valida el ledger sobre la cuenta caliente, con identificador sustituido. No modifica índices ni invariantes.

`pool-info.go` analiza la configuración con la versión de pgxpool ya usada por Benefits, sin conectarse ni imprimir el DSN. `pool.json` se obtuvo pasando el DSN únicamente por entorno y comparando CPU del host y Docker; no es una captura de `Pool.Stat()` de la instancia. Las conexiones/esperas de la instancia se observan por `pg_stat_activity`.

El contador de sesiones filtra `benefits_db` y excluye el PID del monitor, pero incluye procesos internos de PostgreSQL y otras conexiones a esa base. Por eso puede superar el máximo del pool; no debe interpretarse como conexiones adquiridas del pool de Benefits.

El muestreo es cada 5 s **más** el tiempo de consultas, incluye contenedores de otros proyectos solo como suma anónima y no demuestra ausencia de esperas breves. Jaeger mantiene el muestreo previo al 100 %. Cada combinación crea nuevos datos en la misma base; el tamaño global y la caché pueden variar según el orden. Es una ejecución local exploratoria, sin repeticiones estadísticas.

Las API keys se generan en memoria y pasan al contenedor mediante una variable de entorno, sin imprimir su valor. Fixtures y archivos de k6 viven en un temporal propio, se eliminan al terminar y las credenciales sintéticas se rotan. Permanecen los movimientos para respetar el ledger: no se borran datos del gimnasio ni se destruyen volúmenes. Los informes conservan agregados, sin claves, cuerpos ni identificadores de cuentas.

k6 y los percentiles son herramientas adicionales aprobadas en D-50; el fundamento académico es capacidad/latencia/límites (teórico pp. 29–30, 86, 101–102 y 143). Parámetros propios aprobados en D-57. [Documentación oficial k6](https://grafana.com/docs/k6/latest/using-k6/scenarios/).
