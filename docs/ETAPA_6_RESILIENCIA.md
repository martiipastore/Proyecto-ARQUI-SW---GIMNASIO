# Etapa 6 — Resiliencia y caída controlada

Fecha: 16/10/2026. Rama: `feature/entrega-2-etapa-06-resiliencia`, desde main que integra etapa 4 por PR #3 (8b0da47). Estado: ejercicio y validaciones completos; código y pruebas en `29465ee`, integración de esta etapa por PR pendiente. La etapa 5 sigue esperando proveedor.

## Alcance y fundamento

Se valida D10 para Partner → Gateway → Benefits → PostgreSQL y HU-29, HU-32, RN-22, RN-26, RN-27, CL-08 y CL-08b. Las protecciones funcionales ya estaban implementadas en etapas 3/4; esta etapa agrega pruebas, el inyector local y evidencia de recuperación. No cambia reglas de negocio, contrato OpenAPI 1.1.1, timeouts ni decisiones de arquitectura.

Teórico: presupuestos, cancelación e idempotencia, pp. 101–105 y 113–116; transacción local, pp. 27–28; trazabilidad, pp. 7–8. El mecanismo concreto del ejercicio reutiliza Go/HTTP y Docker ya presentes. El proxy es un auxiliar de pruebas para generar la pérdida de una respuesta confirmada; no es un servicio del dominio ni una herramienta externa incorporada al stack normal.

D-43 y D-47 conservan 4 s en Benefits, 5 s en Gateway, sin reintentos de escritura a nivel de aplicación en Gateway y recuperación con la misma clave. D-56 aprueba hasta 500 ms adicionales solo para medir de punta a punta, sin ampliar esos contextos ni afirmar un SLA. Ver [ADR-010 v2](adr/ADR-010-v2-resiliencia-validada.md).

## Escenarios y comprobaciones

1. Benefits detenido: 502 UNAVAILABLE con Retry-After; ningún movimiento nuevo; restaurar y recuperar con la misma clave.
2. PostgreSQL pausado: liveness de Benefits 200 y readiness 503; el POST devuelve 503 UNAVAILABLE alrededor de 4 s; al restaurar se consulta el estado durable y se reintenta la misma clave. En esta corrida la falla ocurrió durante autenticación, antes de escribir.
3. Espera antes de Benefits: el proxy no envía el POST, Gateway agota 5 s y devuelve 504 TIMEOUT; la base no cambia. La misma clave se ejecuta una vez al restaurar la ruta directa.
4. Respuesta perdida después de commit: el proxy recibe el 201 completo y registra solo su hash; retiene la respuesta hasta la cancelación de Gateway, que devuelve 504. Antes del reintento la base ya muestra el cuarto movimiento. La misma clave devuelve 201 con Idempotent-Replayed=true y hash idéntico, sin sumar otra vez.

Además, una prueba PostgreSQL real cancela tras preparar movimiento, saldo y resultado idempotente, antes de commit. Comprueba rollback de los tres y reintento/replay correctos. Un test del Gateway cuenta un único POST frente a 503. El test del inyector comprueba que retiene una sola respuesta y que el modo previo no alcanza Benefits.

Una respuesta 503/504 no demuestra en general que no hubo commit: los recuentos descritos corresponden a estos casos concretos. Ante resultado incierto se conserva la clave y se recupera el resultado durable.

## Reproducción local

Con el stack de etapa 4 ya levantado, Docker iniciado y sin otro uso concurrente del gimnasio:

```powershell
./tests/e2e/verify-resilience.ps1
```

El script detiene temporalmente Benefits y pausa la instancia PostgreSQL del gimnasio, que también usan Members y Booking. Usa fixtures sintéticos nuevos, genera una API key temporal y almacena solo su hash. No imprime la clave ni agrega endpoints de prueba. El fixture se conserva para respetar el ledger; una repetición crea otro aislado.

Para las fallas de transporte combina `deploy/docker-compose.yml` con `tests/resilience/docker-compose.yml`; la imagen del proxy se construye desde su carpeta, sin ampliar el contexto ni las exclusiones de las imágenes normales. Su puerto 9090 es interno, sin publicación al host. Los health checks usan los valores ya presentes en Compose.

Un finally intenta restaurar PostgreSQL, Benefits y la URL normal del Gateway y retira el proxy. La evidencia de éxito solo se escribe después de la restauración y verificaciones de privacidad. Para que los logs sobrevivan, se capturan antes de recrear cada contenedor. Las esperas de disponibilidad/exportación de evidencia son acotadas a 60 s, como en el script de observabilidad; no son presupuestos de negocio.

La terminación forzada del proceso o de Docker puede impedir ejecutar finally. En ese caso, desde la raíz del repositorio (usar unpause solo si PostgreSQL quedó pausado):

```powershell
docker compose --env-file .env -f deploy/docker-compose.yml unpause postgres
docker compose --env-file .env -f deploy/docker-compose.yml up -d --no-deps --wait benefits-service api-gateway
docker compose --env-file .env -f deploy/docker-compose.yml -f tests/resilience/docker-compose.yml rm --stop --force resilience-proxy
```

El aviso temporal de contenedor huérfano al restaurar solo el archivo base corresponde al inyector aún presente; el script lo retira por nombre al terminar. No usar borrado de volúmenes para este ejercicio.

## Resultado medido

Medición local del 16/10/2026; tiempos en milisegundos del cliente, con transporte incluido. No son percentiles ni datos de carga de D13.

| Escenario | Respuesta | Duración ms | trace_id |
|---|---|---|---|
| Benefits detenido | 502 UNAVAILABLE | 3370.831 | d541fae3a6ab4f43a6bfd81cc617fd1b |
| PostgreSQL pausado | 503 UNAVAILABLE | 4004.363 | 575f6a98041c46008397d5a1b9483682 |
| Espera antes de Benefits | 504 TIMEOUT | 5010.314 | 4026873a3e464cc6ae33f07387402448 |
| Respuesta perdida después de commit | 504 TIMEOUT | 5004.196 | 96d97f9483554be39e47e3bab7bc1414 |

Resultado final de la cuenta sintética: saldo 400, cuatro movimientos y cuatro operaciones idempotentes. Las tres primeras recuperaciones se ejecutaron y repitieron sin duplicar; en la cuarta, el primer reintento ya fue replay de la respuesta confirmada perdida.

- `go test ./...`, `go vet ./...` y golangci-lint v2.1.6 correctos en Gateway y Benefits; persistencia PostgreSQL real: 24,883 s en la corrida registrada.
- Tests y vet del proxy: `go test tests/resilience/fault-proxy/main.go tests/resilience/fault-proxy/main_test.go` y el comando equivalente con `go vet`: correctos. CI ejecutará también estas comprobaciones desde su job de Gateway; make test/make lint las incluyen y make fmt contempla el auxiliar.
- Ejercicio completo correcto: 11 intentos HTTP, 9 comprobaciones del estado durable, 5 trazas y 14 líneas de logs vinculadas a sus spans. Validación de ausencia de credenciales, cuerpos e identificadores del fixture antes de exportar.
- Restauración comprobada: ruta `http://benefits-service:8083`, trece contenedores healthy, Collector/Jaeger en ejecución y estado agregado del Gateway `ready`. Proxy temporal retirado; ningún volumen borrado.

## Evidencia y límites

- [Resultados, intentos, estados y línea temporal](evidence/etapa6/results.json).
- [Cinco trazas exportadas](evidence/etapa6/traces.json).
- [Logs sanitizados](evidence/etapa6/logs.jsonl).
- [Postmortem del ejercicio](POSTMORTEM.md).

Las trazas de Benefits detenido y demora previa tienen dos spans del Gateway; no se inventan spans de un servicio que no recibió la petición. La pausa tiene además Benefits y autenticación SQL; la confirmación y el replay incluyen transacción PostgreSQL. La exportación conserva la evidencia aunque se pierda la memoria de Jaeger al reiniciar.

No se prueba caída completa del host, corrupción de almacenamiento, saturación sostenida ni proveedor externo. No se agrega circuit breaker ni se declaran alertas activas o métricas exportadas. La siguiente etapa implementable es carga/capacidad/costos (etapa 7); D9 seguirá pendiente hasta recibir proveedor y contrato.
