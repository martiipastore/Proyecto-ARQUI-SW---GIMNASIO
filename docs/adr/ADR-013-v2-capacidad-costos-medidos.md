# ADR-013 v2 — Capacidad explorada y costos de Benefits

| Campo | Valor |
|---|---|
| **Estado** | Aceptado según D-50 y parámetros D-57 a D-60; evidencia local completada |
| **Fecha** | 2026-10-16 |
| **Decisión del TP** | D13 |
| **Reemplaza** | ADR-013, plan inicial conservado como historia |
| **Relacionado con** | RN-22, RN-26, RN-27; D-41, D-49, D-50, D-57 a D-60; ADR-010 v2 y ADR-011 v2 |

## Problema y alternativas

La elección de Oracle ARM Free Tier no demuestra capacidad ni costo cero. El flujo entregado necesitaba mediciones con persistencia, idempotencia y concurrencia reales antes de justificar su escalado. Medir endpoints sin base o agregar réplicas por anticipado no permite observar el bloqueo de cuenta ni el costo del ledger.

Se conserva la decisión de medir Partner → Gateway → Benefits → PostgreSQL con k6 como instrumento adicional aprobado. No se cambia tecnología, contrato, timeouts, pool ni consistencia para obtener mejores resultados.

## Protocolo aprobado y evidencia

100 cuentas, 10 movimientos precargados por cuenta; read/write/replay/mixed sobre una cuenta y sobre 100 cuentas. Calentamiento 30 s; niveles 1/5/10 VU de 60 s; pausa 100 ms. D-59 agrega, tras la matriz, créditos sobre una cuenta sin pausa, con calentamiento 30 s y niveles de 30 s. D-57/D-59 contienen la aprobación de estos parámetros.

[Procedimiento](../../tests/load/README.md), [informe y alternativas](../ETAPA_7_CAPACIDAD_COSTOS.md), [resultados de matriz](../evidence/etapa7/RESULTADOS.md), [experimento sin pausa](../evidence/etapa7/probe/RESULTADOS.md). Los JSON incluyen ambiente, percentiles, tasa de operaciones, errores, recursos y estado final comprobado por SQL. Se excluye calentamiento de las conclusiones de rendimiento, pero se incluyen sus créditos en integridad.

Las 36 fases terminaron sin fallas funcionales ni inconsistencias. Los niveles medidos cumplieron los objetivos iniciales. En la matriz con pausa, mayores p95 entre fases: lecturas 90,82 ms, créditos 146,10 ms y replay 239,12 ms. No son percentiles globales ni SLA.

## Límite observado

La pausa limita teóricamente 10 VU a 100 iteraciones/s antes del tiempo de respuesta. Esa matriz demuestra comportamiento con carga acotada, no máxima capacidad.

Sin pausa, los créditos sobre una cuenta alcanzaron 61,36 / 90,27 / 82,46 req/s con 1/5/10 VU; p95 36,45 / 175,49 / 472,24 ms. A 10 VU el p99 llegó a 1.082,55 ms. Hubo hasta 7 sesiones esperando locks; la cuenta del diagnóstico acumuló 9.964 movimientos y su agregado tardó 9,918 ms. La contención de cuenta y el trabajo creciente de validación explican un límite que más réplicas de Benefits no eliminan por sí solas.

No se fija una capacidad universal de 90 req/s: es una ejecución exploratoria local x86, con otros proyectos y trazas al 100 %, datos crecientes y caché variable. Falta un ensayo prolongado y repetido en ARM. Las sesiones de `pg_stat_activity` incluyen procesos internos y otras conexiones; no son una lectura directa de ocupación del pool.

## Comparación de mejoras

| Alternativa | Relación con la evidencia | Costo y límite |
|---|---|---|
| Estudiar consultas/índices y duración de la transacción, manteniendo invariantes | Historial de la cuenta y esperas observadas justifican medir cada parte antes/después | No exige por sí misma otra VM; sí desarrollo, espacio y costo de mantenimiento de índices. No se implementó ni se midió una mejora |
| Más CPU/RAM de VM | Daría margen a base y servicios compartidos si dominan recursos | Comparación bruta 2 OCPU/12 GB → 4/24: +USD 28,27/mes; no elimina serialización y falta medir su beneficio en destino |
| Más réplicas o conexiones | Podría ayudar con cuentas independientes si proceso/pool limita | No elimina el bloqueo de una cuenta; más presión sobre PostgreSQL puede empeorar el caso. No hay ganancia demostrada para este flujo caliente |

No se selecciona ni implementa una optimización nueva en este ADR. La evidencia favorece investigar primero el tiempo dentro de la transacción y la validación del historial antes de afirmar que réplicas/pool resolverían el caso caliente. Cada cambio concreto se consulta al equipo y debe conservar RN-22/RN-27.

## Dimensionamiento y costos

D-58 fija São Paulo como referencia, USD y equivalente ARS fechado. [Estimación completa](../COSTOS_ENTREGA_2.md) con fuentes oficiales y respuestas públicas conservadas. Octubre: 744 h. Comparación CPU/RAM y 200 GB Balanced, antes de franquicias/impuestos: 2/12 USD 36,77 (ARS 56.441,95); 4/24 USD 65,04 (ARS 99.836,40), a ARS 1.535/USD BNA vendedor billete del 16/10/2026, 17:00.

La documentación Always Free y las bandas PAYG consultadas difieren; verificar beneficios efectivos, región principal y disponibilidad. La tabla bruta no predice una factura cuando aplican franquicias. Tampoco garantiza que el stack final quepa en esas formas. No se crearon recursos ni se eligió la forma mayor.

D-60 aprueba un supuesto de 200 movimientos diarios, no demanda real. Con espacio asignado observado, movimientos más respuestas idempotentes e índices proyectan aproximadamente 9,96 MiB/30 días, excluyendo WAL, backups, otras bases y logs. No existe purga automática de idempotencia; no se presume almacenamiento constante después de 30 días.

## Consecuencias y verificación

Se dispone de pruebas reproducibles, límites explicados y una comparación económica fechada. Se conserva una sola instancia de Benefits y las protecciones existentes. El costo de pruebas es tiempo y crecimiento de datos sintéticos locales; credenciales rotadas y temporales eliminados, sin borrar el ledger.

Antes de desplegar: verificar disponibilidad/elegibilidad, configuración del pool y recursos visibles en ARM, almacenamiento de todo el stack, backup/restauración y tráfico. No se extrapolan directamente resultados de 8 CPU locales a 2/4 OCPU.

Fundamento académico: capacidad, latencia y costos, pp. 29–30, 86, 101–102 y 143; transacciones/invariantes, pp. 27–28. k6 y percentiles son instrumentos adicionales aprobados en D-50. Pausas, cargas, umbrales, región y demanda supuesta son elecciones propias consultadas; no se atribuyen al teórico.
