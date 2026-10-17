# Etapa 7 — Capacidad y costos de Benefits

Estado: **completada e integrada en `main`** mediante el PR #5 (`fb2a17c`), 16/10/2026. Etapa 5 sigue pendiente de asignación del proveedor externo; esta prueba no sustituye esa integración.

## Alcance y decisiones

Flujo real Partner → Gateway → Benefits → PostgreSQL por el contrato público, sin modificar código productivo. Requisitos HU-29/HU-32, RN-22/RN-26/RN-27, CL-08b y D13. Se conserva la autorización ya aprobada de k6 como herramienta adicional; no se incorpora otra dependencia de ejecución.

- D-50: 100 cuentas, escalones de 1/5/10 VU y objetivos iniciales p95 de lecturas < 200 ms y escrituras < 500 ms; cero errores funcionales inesperados y duplicaciones.
- D-57: pausa inicial de 100 ms y 10 movimientos precargados por cuenta. Son valores propios consultados al equipo; el teórico no fija estos números.
- D-58: São Paulo como referencia de nube, USD y conversión informativa a ARS con fecha/fuente. No se crean recursos.
- D-59: si la pausa impide observar saturación, experimento posterior de créditos sobre una cuenta, sin pausa, con 1/5/10 VU durante 30 s por nivel; conservar calentamiento y datos iniciales. Evidencia separada y sin cambios productivos.
- D-60: supuesto ilustrativo de 200 movimientos diarios para proyectar crecimiento con tamaños medidos. No es demanda real confirmada; no se supone una purga de idempotencia que todavía no existe.

Fundamento de clase: capacidad, latencia, límites y costos, pp. 29–30, 86, 101–102 y 143 del PDF. k6 y percentiles son instrumentos adicionales aprobados para cumplir las pruebas del TP. Un percentil p95 no es promedio ni máximo; indica un tiempo que no supera el 95 % de las observaciones de esa fase.

## Ambiente y procedimiento

[Procedimiento reproducible](../tests/load/README.md), [ambiente capturado](evidence/etapa7/environment.json), [imágenes y versión de código](evidence/etapa7/images.json) y [configuración del pool](evidence/etapa7/pool.json). Windows, Docker Desktop x86_64 con 8 CPU lógicas y aproximadamente 3,7 GiB de RAM, recursos compartidos con otros contenedores. Sin despliegue ARM ni medición por Internet. Jaeger sigue con muestreo al 100 % y memoria según D-55.

La configuración del pool se analizó con el mismo pgxpool usado por Benefits: máximo 8, mínimo 0, vida máxima 1 h, inactividad máxima 30 min y comprobación cada 1 min. El diagnóstico corre en el host, cuya cantidad de CPU coincide con Docker; las conexiones reales se muestrean en PostgreSQL. No se cambió la configuración para mejorar un resultado.

El máximo predeterminado de pgxpool es `max(4, NumCPU)`: no se traslada automáticamente ese 8 a una VM con 2/4 OCPU. El dimensionamiento de nube debe comprobar el número de CPU visibles y el pool efectivo en destino antes de comparar capacidad.

Ocho combinaciones: read/write/replay/mixed × hot/distributed. Nuevo fixture por combinación, 100 cuentas y 1.000 créditos iniciales de 1 punto; calentamiento 30 s y escalones de 60 s. Mezcla 80/20, lecturas alternadas de saldo/movimientos, replay comparado con respuesta confirmada. Datos acumulados entre fases de la misma combinación; la base global acumula fixtures entre combinaciones. Son 28 minutos activos más precarga y recolección.

## Evidencia y comprobaciones

[Tabla generada de resultados](evidence/etapa7/RESULTADOS.md) a partir de JSON por fase. Se excluye calentamiento del análisis de rendimiento, pero se incluyen sus escrituras en la validación de integridad. Cada fase compara saldos, secuencias, movimientos, operaciones idempotentes y créditos nuevos confirmados. Los umbrales fallidos deben conservarse como fallos, no omitirse del reporte. El percentil de lecturas agrupa saldo y movimientos; no permite afirmar el percentil individual de cada endpoint.

El muestreo de recursos tiene un intervalo objetivo de 5 s más consultas; una espera breve puede no aparecer. No se dispone de historial continuo de espera del pool ni atribución estadística por consulta. Otros proyectos se registran solo mediante totales de recursos, sin nombres o identificadores.

## Resultados de la matriz con pausa

Las 32 fases terminaron con tasa funcional 0 y todas las invariantes verificadas. Los 24 niveles medidos (sin calentamiento) cumplieron sus objetivos. El mayor p95 por tipo entre esos niveles fue: lecturas **90,82 ms**, créditos nuevos **146,10 ms** y replay **239,12 ms**. Son máximos entre percentiles de fases, no un percentil global.

Con 10 VU: lecturas 89,45 req/s sobre una cuenta y 84,70 distribuidas; créditos 73,04 y 74,92; replay 78,54 y 82,23; mezcla 84,10 y 74,60. La mezcla conserva el throughput conjunto, no una tasa independiente para cada operación.

Se observaron hasta 7 sesiones esperando locks en créditos concentrados, frente a 1 en créditos distribuidos; replay concentrado llegó a 4. La implementación usa bloqueo de cuenta incluso al revalidar la vinculación para replay. Estos máximos son muestras, no el tiempo total de bloqueo ni una atribución exacta por sentencia.

En créditos concentrados, PostgreSQL llegó a 95,77 % de CPU Docker (aproximadamente un núcleo). En toda la matriz otros contenedores alcanzaron un agregado de 446,93 %, y Jaeger llegó a 211,46 % y 302,90 MiB. Son máximos independientes, no necesariamente simultáneos. La carga de fondo y el muestreo al 100 % impiden tratar estos números como capacidad aislada de Benefits.

## Interpretación y límite explorado

La pausa fija establece un techo del generador de 10 iteraciones/s por VU, antes de agregar tiempo de respuesta. Estar cerca de ese techo prueba comportamiento bajo esa carga; no identifica por sí solo la saturación de Benefits. Se debe distinguir un límite observado de una hipótesis pendiente de experimento.

El plan `ledger-plan.json` de cada combinación observa el agregado que participa en la invariante del ledger, sin cambiar índices. Es diagnóstico, no una demostración del costo completo de una transacción. Un resultado favorable local no garantiza igual latencia en una VM ARM o con un historial de meses.

Después de créditos concentrados, el agregado examinó 7.497 filas de esa cuenta y tardó 2,632 ms; en créditos distribuidos, 99 filas y 1,004 ms. Distinto tamaño y estado de caché: no es un ensayo controlado de índices. La lectura del SQL confirma que la validación suma el historial; los planes justifican estudiar su costo al crecer, sin debilitar la consistencia.

Como la matriz con pausa no alcanzó una saturación demostrada, se ejecutó el experimento adicional D-59: misma API y protecciones, créditos sobre una cuenta, sin pausa, precarga propia y [evidencia separada](evidence/etapa7/probe/RESULTADOS.md). No se extrapola linealmente a más usuarios ni se amplía concurrencia sin aprobación.

| Usuarios sin pausa | Créditos/s | p95 ms | p99 ms |
|---:|---:|---:|---:|
| 1 | 61,36 | 36,45 | 64,01 |
| 5 | 90,27 | 175,49 | 360,46 |
| 10 | 82,46 | 472,24 | 1.082,55 |

El throughput dejó de crecer al pasar de 5 a 10 VU, mientras aumentó la latencia. Se observaron hasta 7 sesiones esperando locks, y PostgreSQL llegó a 181,43 % de CPU Docker. Se identifica **contención de las transacciones sobre una cuenta compartida**, junto con mayor trabajo de validación al crecer el historial, como límite concreto del flujo explorado. La base terminó con 10.954 movimientos/operaciones y la misma suma de saldos; el agregado de la cuenta examinó 9.964 filas y tardó 9,918 ms.

Esto no fija una capacidad máxima universal de 90 req/s: cambian el historial entre fases, la caché y la carga del equipo. No hay un ensayo de duración prolongada ni repeticiones estadísticas. El p95 de 10 VU aún cumple < 500 ms, pero el p99 supera 1 s: cumplir p95 no garantiza que todas las respuestas estén por debajo de 500 ms.

El contador de `pg_stat_activity` alcanzó 9 sesiones en `benefits_db`: incluye procesos internos y otras conexiones además del pool, excluyendo el monitor. No contradice por sí solo el máximo 8 del pool ni representa su ocupación exacta. No se capturó `backend_type` para atribuir esa sesión adicional.

## Comparación inicial de alternativas

Estas son alternativas para analizar con evidencia; no cambios ya aprobados o implementados.

| Alternativa | Qué podría mejorar | Límite y comprobación requerida |
|---|---|---|
| Optimizar consultas e índices del ledger, conservando invariantes | El plan con 9.964 filas y las esperas justifican estudiar trabajo y duración de la transacción; no añade una tarifa de infraestructura por sí mismo | No elimina la serialización sobre una cuenta; requiere desarrollo y medir antes/después, incluyendo espacio y costo de cada índice. No quitar triggers de consistencia |
| Aumentar CPU/RAM de la VM | Más margen para base y servicios compartidos; comparación bruta 2/12 → 4/24: +USD 28,27/mes antes de franquicias | No resuelve automáticamente locks ni consultas que crecen con el ledger; confirmar en ARM. No se midió el beneficio de la forma mayor |
| Replicar Benefits o aumentar el pool | Puede ayudar entre cuentas si el proceso/pool limita y PostgreSQL tiene margen | Las 7 esperas de la cuenta caliente seguirían en la base. Más conexiones pueden empeorarla; no es la primera mejora demostrada para este caso. Costo depende de si necesita otra VM; no hay experimento antes/después |

La comparación de recursos 2 OCPU/12 GB frente a 4 OCPU/24 GB y sus costos está en [la estimación fechada](COSTOS_ENTREGA_2.md). No implica seleccionar la forma mayor. No se promete que todo el stack futuro quepa en la forma gratuita.

## Conclusión y validación

Las 36 fases (32 de matriz y 4 adicionales, incluidos calentamientos) conservaron las invariantes y no tuvieron errores funcionales; todos los niveles medidos cumplieron los objetivos iniciales. La matriz con pausa está acotada por el generador; el experimento sin pausa muestra contención y crecimiento de latencia sobre una cuenta. No se cambió una protección para obtener mejores números ni se aplicó una optimización sin consultar.

El espacio observado y la proyección aprobada D-60 se conservan en [storage.json](evidence/etapa7/storage.json), [storage-projection.json](evidence/etapa7/storage-projection.json) y [costos](COSTOS_ENTREGA_2.md). El gateway volvió a informar `ready` después de la carga. Tests y `go vet` de los siete módulos correctos (paquetes reutilizados de la caché de Go); inyector y diagnóstico del pool correctos; Spectral sin advertencias. Las pruebas k6/SQL se ejecutaron realmente en esta etapa. El frontend se omitió por ausencia de `node_modules`, como prevé el Makefile; golangci-lint no está instalado localmente y queda en CI. Sintaxis PowerShell/Python y JSON/XML/ZIP verificados. Detalle: [validation.json](evidence/etapa7/validation.json). Commit local de etapa e integración por PR se registran en el plan.
