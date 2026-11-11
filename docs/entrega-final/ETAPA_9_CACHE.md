# Etapa 9 — Caché relevante

**Estado:** verificación técnica local cerrada el 10/11/2026. El E2E `-cache`
aprobó **24 escenarios**: la regresión de identidad, membresías y Booking más
4 de caché. Evidencia y mediciones en
[cache-e2e.json](../evidence/entrega-final/etapa9/cache-e2e.json). Falta la
revisión de otro integrante. No se publicó en Vercel.

Diseño: [ADR-007](../adr/ADR-007-cache-autorizacion-disponibilidad.md). Decisiones
del equipo del 10/11/2026 en **D-88**: catálogo de actividades, cliente go-redis v9
y pool de conexiones del gateway.

## 1. Elección con mediciones

ADR-007 permite cachear tres catálogos. El de beneficios no existe (D-76). Se
midieron los otros dos por el gateway, con JWT real: 10 clientes en bucle cerrado
durante 20 s después de 3 s de calentamiento.

| Línea base sin caché | req/s | p50 | p95 | p99 | Lecturas de la tabla por solicitud |
|---|---|---|---|---|---|
| `GET /activities` (Booking) | 1.753 | 4,96 ms | 10,91 ms | 16,68 ms | 1,00 |
| `GET /memberships/types` (Members) | 1.586 | 4,85 ms | 13,70 ms | 27,89 ms | 1,00 |

Los dos leen la base en cada solicitud y tienen latencias parecidas. El equipo
eligió **actividades**:

- se lee cada vez que un alumno abre Clases y en la pantalla del administrador,
  así que es el catálogo más pedido del recorrido principal (D-85);
- descarga `booking_db`, la base de reservas y cupos.

### Hallazgo previo: el gateway agotaba los puertos

La primera medición falló. Los tipos de membresía devolvían 8.802 respuestas 502
en 20 s. El gateway usaba `http.DefaultTransport`, que conserva **2** conexiones
ociosas por servicio. Con 10 solicitudes simultáneas abría y cerraba conexiones
sin parar. Después de ~20.000 solicitudes a actividades, Windows se quedó sin
puertos efímeros (TIME_WAIT) y Members se volvió inaccesible.

Se corrigió con un transporte propio (`proxy.NewTransport`): 64 conexiones ociosas
por servicio, 256 en total y 90 s de vida (D-88). Hay un test que verifica que 10
clientes concurrentes no abren más de 10 conexiones.

| `GET /activities`, misma carga | req/s | p95 |
|---|---|---|
| Gateway con transporte por defecto | 1.018 | 20,08 ms |
| Gateway con pool de 64 | 1.753 | 10,91 ms |

Esta corrección es la mejora de rendimiento más grande de la etapa y afecta a
todos los servicios detrás del gateway.

## 2. Implementación

- **Cache-aside** en `application.Servicio.Actividades` (puerto `CacheActividades`):
  un acierto no consulta PostgreSQL; un miss lee la base y guarda la copia.
- **Invalidación** después de confirmar un alta o una modificación de actividad.
  Un rechazo (validación, permiso) no invalida.
- **TTL de 10 min** (ADR-007) en la clave versionada `booking:actividades:v1`,
  como JSON con las etiquetas en el adaptador (`adapters/redis`), nunca en `domain`.
- **Redis caído:** cada operación tiene un timeout de 100 ms y la lectura cae a
  PostgreSQL. Escribir actividades sigue funcionando. Readiness informa `degraded`
  (Redis es dependencia parcial) y `/metrics` cuenta el error.
- **Qué no se cachea:** nada que decida permisos ni reservas. Generar clases,
  reservar e ingresar leen PostgreSQL (ADR-007, RN-12/14).
- **Métricas** en `/metrics` de booking-service: aciertos, fallos, errores,
  invalidaciones, cantidad de operaciones y tiempo acumulado en Redis.
- **Configuración:** `BOOKING_CACHE_ENABLED` (apagado por defecto),
  `BOOKING_REDIS_ADDR`, `BOOKING_REDIS_PASSWORD`, `BOOKING_CACHE_TTL` (10m) y
  `BOOKING_CACHE_TIMEOUT` (100ms). En Compose usa el `redis` existente con
  `REDIS_PASSWORD`.

## 3. Comparación antes y después con la misma carga

Para que el ruido de la máquina no se confunda con el efecto de la caché, el E2E
alterna en la misma corrida **sin caché / con caché / sin caché / con caché**,
reiniciando Booking en cada ronda. Los tipos de membresía, que no tienen caché,
se miden como control en cada ronda.

| Ronda | Actividades req/s | p95 | Lecturas de base por solicitud | Control req/s |
|---|---|---|---|---|
| 1 sin caché | 1.402 | 13,62 ms | 1,00 | 1.354 |
| 2 con caché | 1.336 | 13,90 ms | **0,00** | 1.326 |
| 3 sin caché | 1.155 | 17,76 ms | 1,00 | 1.427 |
| 4 con caché | 1.277 | 16,49 ms | **0,00** | 1.451 |

Valores de la corrida del 10/11/2026; la evidencia JSON guarda los promedios por modo.

**Conclusión:**

- La caché **elimina el 100% de las lecturas del catálogo a `booking_db`**: más de
  30.000 aciertos y 0 fallos en las rondas con caché.
- **No mejora la latencia de forma medible.** Las diferencias entre modos (±5%)
  quedan dentro de la variación del control. Cada operación contra Redis tardó en
  promedio ~4,3 ms bajo esta carga, lo mismo que la consulta a PostgreSQL de un
  catálogo de 5 filas. En una sola máquina, el tiempo lo dominan el recorrido HTTP,
  la verificación del JWT y el proxy, no la base.
- El beneficio real es **sacar carga de la base transaccional**. Con más tráfico o
  con la base en otra máquina, las lecturas evitadas liberan conexiones para reservas.

Son mediciones locales, sin garantía de producción. No se atribuyen al teórico.

## 4. Límites

- **Carrera de cache-aside:** una lectura que trajo el catálogo viejo justo antes de
  una escritura puede guardarlo después de la invalidación. La copia vieja dura
  como máximo el TTL de 10 min. Es aceptable porque el catálogo no decide permisos
  ni reservas.
- **Escritura con Redis caído:** si Redis está caído al modificar una actividad, la
  invalidación falla y la copia anterior puede reaparecer al volver Redis, hasta el
  TTL. El E2E lo muestra y lo deja documentado. Se podría acortar con un TTL menor
  si el equipo lo prefiere.
- La medición usa el generador en Go del E2E, no k6: los servicios aislados del E2E
  escuchan en el loopback de la máquina y un contenedor de k6 no llega de forma
  confiable. Los scripts k6 de la Entrega 2 siguen para el stack de Compose.

## 5. Verificación

```text
go run ./tests/e2e/identity/main.go -cache
```

Levanta PostgreSQL, Mailpit y Redis aislados, corre la regresión de identidad,
membresías y Booking, y después:

- la medición alternada con control;
- métricas con tasa de aciertos > 99%;
- la invalidación visible en la siguiente lectura;
- Redis caído: catálogo desde PostgreSQL sin errores, readiness `degraded`,
  escrituras que siguen, y recuperación con nuevos aciertos.

Tests:

- unitarios de aplicación: miss/acierto, invalidación solo al confirmar, Redis
  caído, base caída;
- `TestActividadesRedisReal` contra Redis 7: TTL, invalidación, copia ilegible,
  métricas y timeout;
- `TestTransportReutilizaConexiones` del gateway.
