# Estimación de infraestructura — 16/10/2026

Referencia aprobada D-58: Oracle Cloud, São Paulo (`sa-saopaulo-1`), USD y equivalente ARS. La región identifica dónde estarían los servidores; los usuarios pueden estar en Córdoba. No se crearon recursos. [Regiones oficiales](https://docs.oracle.com/en-us/iaas/Content/General/Concepts/regions.htm).

## Precios y cálculo reproducible

Se conserva la respuesta pública de cada SKU con URL y fecha en [pricing-sources.json](evidence/etapa7/pricing-sources.json). Tarifas USD de la lista pública, sin descuentos negociados. Última actualización informada por la API: 08/10/2026. No es una oferta reservada ni se verificó disponibilidad de la cuenta en esa región.

| Recurso | SKU | Precio unitario público USD | Fuente oficial |
|---|---|---:|---|
| A1, CPU ARM | B93297 | 0,01 / OCPU-h fuera de franquicia | [API Oracle](https://apexapps.oracle.com/pls/apex/cetools/api/v1/products/?partNumber=B93297&currencyCode=USD) |
| A1, memoria | B93298 | 0,0015 / GB-h fuera de franquicia | [API Oracle](https://apexapps.oracle.com/pls/apex/cetools/api/v1/products/?partNumber=B93298&currencyCode=USD) |
| Volumen, capacidad | B91961 | 0,0255 / GB-mes | [API Oracle](https://apexapps.oracle.com/pls/apex/cetools/api/v1/products/?partNumber=B91961&currencyCode=USD) |
| Rendimiento del volumen | B91962 | 0,0017 / VPU por GB-mes | [API Oracle](https://apexapps.oracle.com/pls/apex/cetools/api/v1/products/?partNumber=B91962&currencyCode=USD) |

Comparación de dimensionamiento, sin elegir ni contratar una máquina: octubre tiene 31 × 24 = **744 horas**. Para hacer comparable el disco se supone un total combinado de 200 GB de volúmenes con 10 VPU/GB (Balanced): `200 × (0,0255 + 10 × 0,0017) = USD 8,50/mes`. Los 200 GB son un escenario de presupuesto, no una necesidad medida del gimnasio. [Definición oficial de Balanced](https://docs.oracle.com/en-us/iaas/Content/Block/Concepts/blockvolumeperformance.htm).

| Escenario bruto, antes de franquicias | CPU/mes USD | RAM/mes USD | Disco/mes USD | Subtotal mensual USD | Equivalente ARS |
|---|---:|---:|---:|---:|---:|
| A1, 2 OCPU / 12 GB | 14,88 | 13,39 | 8,50 | **36,77** | **56.441,95** |
| A1, 4 OCPU / 24 GB | 29,76 | 26,78 | 8,50 | **65,04** | **99.836,40** |

Fórmula CPU/RAM: `744 × (OCPU × 0,01 + GB_RAM × 0,0015)`. Redondear cada subtotal mensual a dos decimales; luego convertir. La segunda forma cuesta USD 28,27 adicionales en esta comparación bruta. No demuestra el doble de capacidad ni elimina bloqueos sobre una cuenta.

Conversión informativa: **ARS 1.535 por USD**, BNA billete vendedor del **16/10/2026, 17:00**. `ARS = USD redondeados × 1.535`. No representa el cargo final de tarjeta ni incluye impuestos, percepciones o comisiones. [Cotización BNA](https://www.bna.com.ar/Personas).

## Franquicias: costo potencial, no garantía

La documentación consultada de Always Free informa para tenencias gratuitas A1 **1.500 OCPU-h y 9.000 GB-h mensuales**, equivalentes a 2 OCPU/12 GB, y 200 GB combinados de boot/block; exige región principal y está sujeta a capacidad. Así, el primer escenario podría quedar en USD 0 para esos recursos si la cuenta cumple condiciones y no consumió sus franquicias. La documentación incluye cinco backups de volumen gratuitos. No se implementó todavía una política de backup y restauración. [Always Free](https://docs.oracle.com/en-us/iaas/Content/FreeTier/freetier_topic-Always_Free_Resources.htm).

La respuesta de precios PAYG, en cambio, publica bandas gratuitas de 3.000 OCPU-h y 18.000 GB-h. No se asume que esas bandas correspondan a toda tenencia gratuita: verificar modalidad de cuenta y beneficios efectivos antes de provisionar. Los subtotales de la tabla permiten comparar recursos **sin aplicar franquicias**; no predicen una factura cuando estas están disponibles. [Lista de precios y condiciones](https://www.oracle.com/cloud/price-list/).

## Alcance y costos que faltan dimensionar

La VM alojaría el stack compartido del proyecto; no es un precio exclusivo de Benefits. Las mediciones locales son x86 y no validan que el stack final, incluyendo OpenSearch y observabilidad completa, quepa en estas formas ARM. Se requiere una prueba en la forma elegida antes de confirmar D-41 como despliegue suficiente.

| Componente | Tratamiento de esta estimación |
|---|---|
| PostgreSQL, Redis, MongoDB, RabbitMQ y servicios | Contenedores dentro de la VM; consumen sus recursos y disco, sin contratar bases administradas |
| Observabilidad actual | Collector y Jaeger en la misma VM hipotética; trazas en memoria según D-55, sin retención productiva garantizada |
| Logs, métricas y trazas futuras | Retención y volumen de ingestión aún no definidos; el disco presupuestado se comparte, no equivale a almacenamiento ilimitado |
| Backups | Capacidad gratuita sujeta a condiciones; política, restauración y excedentes no dimensionados |
| Tráfico saliente | Oracle publica primeros 10 TB/mes sin cargo; uso del proyecto aún no pronosticado; verificar ruta y condiciones aplicables |
| Acceso público / TLS | Sin balanceador administrado en este escenario (Traefik en la VM); dominio no elegido ni cotizado; verificar condiciones de IP y certificados al desplegar |
| Impuestos / pago / tipo de cambio | Excluidos del subtotal técnico; la conversión ARS es una referencia fechada |
| Trabajo operativo | Sin cotización de horas de administración, soporte o mantenimiento |

Fuente de red: [Oracle Networking](https://www.oracle.com/cloud/networking/virtual-cloud-network/pricing/). El subtotal CPU/RAM/disco no se presenta como costo total de producción.

El crecimiento se proyecta con `movimientos/día × bytes por movimiento`, más respuestas idempotentes, índices, otras bases, WAL y backups. D-60 aprueba un supuesto ilustrativo de **100 cuentas × 2 movimientos/día = 200 movimientos/día**: 6.000 movimientos en una ventana de 30 días. Se aplicará el tamaño medido al finalizar la carga; no representa demanda real confirmada.

RN-27 exige conservar idempotencia al menos 30 días. La implementación actual impide borrarla antes de ese plazo, pero no tiene purga automática; por eso ambos conjuntos continúan creciendo. No se presupuesta una meseta de 6.000 respuestas ni se propone borrar el ledger.

Medición posterior a las cargas: 38.847 movimientos y 38.847 operaciones en toda la base local (incluye fixtures anteriores). Ledger: 13.877.248 bytes con índices, aproximadamente **357,23 bytes/fila**; idempotencia: 53.747.712 bytes con índices, aproximadamente **1.383,57 bytes/fila**. Base completa: **73,16 MiB**. [Consulta](../tests/load/storage.sql), [medición](evidence/etapa7/storage.json) y [cálculo](evidence/etapa7/storage-projection.json).

Con D-60, el crecimiento ilustrativo conjunto es `6.000 × (357,23 + 1.383,57) / 1.048.576 ≈ 9,96 MiB por 30 días`, o aproximadamente 121,19 MiB por 365 días sin purga. Se calculó con valores sin redondear. Es espacio asignado por fila observado: incluye índices, espacio libre y versiones muertas; no es costo marginal exacto. Cuerpos reales, índices, otras bases, WAL, logs, backups y retención cambian el resultado. Esta cifra no dimensiona por sí sola el disco de todo el stack.

## Fundamento académico

Costos, capacidad y límites: teórico pp. 29–30 y 143. Los precios oficiales, Oracle, región, conversión monetaria y formas comparadas son datos y elecciones prácticas del proyecto; no valores enseñados por el material. D-58 aprueba el escenario de estimación, sin ampliar el alcance a una contratación.
