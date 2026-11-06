# Guía_Entrega2

Guía breve para preparar y demostrar la Entrega 2 de METALFITNESS. El recorrido prueba el servicio operativo, la capacidad propia, PostgreSQL, idempotencia, observabilidad y resiliencia. La integración externa D9 no forma parte de esta demostración porque continúa pendiente de la asignación del proveedor.

## 1. Preparación

Requisitos:

- Docker Desktop iniciado;
- PowerShell abierto en la raíz del repositorio;
- puertos 4010, 5173, 5432, 5672, 6379, 8080–8086, 15672, 16686 y 27017 disponibles;
- rama o `main` actualizado con los cambios de la Entrega 2.

Crear la configuración local solamente si todavía no existe:

```powershell
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
```

`.env` está ignorado por Git. Sus valores son exclusivamente para desarrollo local.

Construir y levantar el sistema completo:

```powershell
docker compose --env-file .env -f deploy/docker-compose.yml up -d --build --wait
```

Comprobar los contenedores:

```powershell
docker compose --env-file .env -f deploy/docker-compose.yml ps
```

Resultado esperado: trece componentes con health check saludable; Jaeger y OpenTelemetry Collector en ejecución.

## 2. Presentación inicial

Explicar brevemente:

1. Benefits es el servicio operativo elegido para la Entrega 2.
2. La capacidad propia se publica para partners en `/partner-api/v1` mediante el Gateway.
3. Benefits utiliza arquitectura hexagonal y PostgreSQL.
4. La acreditación confirma movimiento, saldo y resultado idempotente en una transacción local.
5. Gateway aplica un timeout de 5 segundos y Benefits uno de 4 segundos.

Documentos de apoyo:

- [arquitectura general](../ARCHITECTURE.md);
- [contrato de partners](../contracts/README.md);
- [validación técnica](VALIDACION_ENTREGA_2.md);
- [decisiones arquitectónicas](../adr/).

## 3. Estado del sistema

Abrir:

- Frontend: <http://localhost:5173>;
- estado agregado: <http://localhost:8080/api/v1/status>;
- Jaeger: <http://localhost:16686>.

También puede consultarse el estado desde PowerShell:

```powershell
Invoke-RestMethod http://localhost:8080/api/v1/status | ConvertTo-Json -Depth 8
```

Resultado esperado: estado global `ready` y los seis procesos internos disponibles.

## 4. Flujo principal de Benefits

Ejecutar el verificador con evidencia temporal para no reemplazar los archivos versionados:

```powershell
$evidence = Join-Path $env:TEMP ("metalfitness-demo-" + [guid]::NewGuid().ToString("N"))
./tests/e2e/verify-benefits.ps1 -Observability -EvidenceDirectory $evidence
```

El script crea un partner y una cuenta sintéticos, guarda únicamente el hash de la API key y verifica a través del Gateway:

1. acreditación: HTTP 201;
2. repetición con la misma `Idempotency-Key`: HTTP 201 y cuerpo idéntico;
3. saldo: 100 puntos;
4. movimientos: un movimiento;
5. misma clave con otro contenido: HTTP 409;
6. credencial inválida: HTTP 401;
7. cuenta no vinculada: HTTP 404.

Puntos para explicar:

- el replay no vuelve a sumar puntos;
- el resultado original queda almacenado junto con la clave;
- una restricción única y el bloqueo transaccional protegen la concurrencia;
- los datos se conservan en PostgreSQL, no en memoria del proceso.

## 5. Logs y traza distribuida

El comando anterior imprime un `trace_id`. En Jaeger:

1. elegir `api-gateway`;
2. buscar la operación de créditos o la traza por su identificador;
3. mostrar los spans de Gateway, cliente HTTP, Benefits, autenticación PostgreSQL y transacción PostgreSQL;
4. señalar que los logs comparten `correlation_id` y `trace_id`;
5. explicar que no se exportan API keys, cuerpos, identificadores de usuario ni sentencias SQL.

Para observar los logs recientes:

```powershell
docker compose --env-file .env -f deploy/docker-compose.yml logs --since 10m api-gateway benefits-service
```

Jaeger conserva sus datos en memoria. La evidencia estable de la etapa está en `docs/evidence/etapa4`.

## 6. Resiliencia

Esta prueba provoca fallas temporales. Debe ejecutarse sin otra demostración o uso concurrente del gimnasio:

```powershell
$evidence = Join-Path $env:TEMP ("metalfitness-resilience-" + [guid]::NewGuid().ToString("N"))
./tests/e2e/verify-resilience.ps1 -EvidenceDirectory $evidence
```

Resultado esperado:

- 502 con Benefits detenido;
- 503 con PostgreSQL pausado;
- 504 por demora antes de Benefits;
- 504 por respuesta perdida después del commit;
- recuperación con la misma clave sin duplicación;
- saldo final 400, cuatro movimientos y cuatro operaciones;
- stack restaurado y proxy temporal eliminado.

La diferencia importante es que un timeout no permite saber si hubo commit. El partner debe conservar y reutilizar la misma clave para recuperar el resultado durable.

Si el script fuera interrumpido, restaurar el entorno con:

```powershell
docker compose --env-file .env -f deploy/docker-compose.yml unpause postgres
docker compose --env-file .env -f deploy/docker-compose.yml up -d --no-deps --wait benefits-service api-gateway
docker compose --env-file .env -f deploy/docker-compose.yml -f tests/resilience/docker-compose.yml rm --stop --force resilience-proxy
```

## 7. Evidencias y decisiones para mostrar

| Tema | Documento |
|---|---|
| Dominio y PostgreSQL | [ETAPA_2_BENEFITS.md](ETAPA_2_BENEFITS.md) |
| API y Gateway | [ETAPA_3_API_BENEFITS.md](ETAPA_3_API_BENEFITS.md) |
| Logs y trazas | [ETAPA_4_OBSERVABILIDAD.md](ETAPA_4_OBSERVABILIDAD.md) |
| Resiliencia | [ETAPA_6_RESILIENCIA.md](ETAPA_6_RESILIENCIA.md) y [POSTMORTEM.md](../POSTMORTEM.md) |
| Capacidad y costos | [ETAPA_7_CAPACIDAD_COSTOS.md](ETAPA_7_CAPACIDAD_COSTOS.md) |
| Validación de cierre | [VALIDACION_ENTREGA_2.md](VALIDACION_ENTREGA_2.md) |
| Seguimiento completo | [PLAN_ENTREGA_2.md](PLAN_ENTREGA_2.md) |

Al presentar las decisiones, diferenciar diseño de implementación:

- D2, D3, D8, D10 y la primera versión de D11 tienen código o evidencia ejecutable.
- D6, D7 y D12 son decisiones de diseño requeridas en esta entrega; su implementación pertenece a etapas posteriores del TP.
- D13 tiene mediciones locales y una estimación de costos con límites explícitos.
- D9 permanece propuesta y bloqueada; no se presenta como integración terminada.

## 8. Finalización

Para detener el sistema conservando los datos:

```powershell
docker compose --env-file .env -f deploy/docker-compose.yml down
```

No usar `down -v` ni `make clean` durante la demostración: esos comandos eliminan los volúmenes locales.

