# ADR-009 v2 — Acreditar puntos en MELT desde Benefits por cada asistencia

| Campo | Valor |
|---|---|
| **Estado** | Aceptado por el equipo el 11/11/2026 (flujo, cantidad y canal del enlace); implementado y validado contra MELT real con la clave |
| **Fecha** | 2026-11-11 |
| **Decisión del TP** | D9 |
| **Reemplaza** | [ADR-009](ADR-009-consumo-proveedor.md) |
| **Autores** | Equipo del TPI |
| **Relacionado con** | D-92; ADR-002 v2; ADR-005 v2; ADR-007; ADR-010 v2; [ETAPA_13_MELT.md](../entrega-final/ETAPA_13_MELT.md) |

## Contexto

ADR-009 quedó propuesto porque el manual de MELT del 10/11/2026 solo describía su
catálogo, el registro referido y su login, sin un flujo que afectara al gimnasio. El
11/11/2026 MELT publicó la API **«Acreditación externa de puntos» v1.1.0**
(OpenAPI versionado en
[docs/contracts/providers/melt](../contracts/providers/melt/acreditar-puntos-v1.openapi.yaml)):
- el consumidor decide cuántos puntos otorga;
- envía `email` o `usuarioId`, `puntos` y una `referencia` única con `X-Partner-Key`;
- MELT responde `201`/`200` (acreditado, idempotente), `202` (pendiente con
  `registroUrl` que el consumidor entrega en privado), `4xx` definitivos o `503`/timeout
  (reintentar con la misma referencia).

El enunciado (§4) exige incorporarla en un flujo importante, llamarla desde el
microservicio responsable (no desde el frontend ni el gateway), definir timeouts y
errores, decidir qué ve el usuario y tener un test de contrato.

## Alternativas consideradas

| Alternativa | Ventaja | Desventaja |
|---|---|---|
| Llamar a MELT dentro de la transacción de la asistencia | Simple | Una caída o una demora de MELT (Render tarda ~25 s en despertar) bloquearía o revertiría la asistencia y los puntos locales |
| Llamar a MELT desde el consumidor AMQP, antes del ACK | Reutiliza los reintentos y la DLQ del broker | Ata la entrega del evento a un tercero y un error definitivo de MELT terminaría en la DLQ como si el evento fuera inválido |
| **Registrar el envío en la misma transacción y entregarlo en segundo plano** | La asistencia y los puntos locales nunca dependen de MELT; el envío no se pierde ni se duplica; reintentos propios con la misma referencia | Una tabla y un proceso más; el alumno ve los puntos MELT con demora |

## Decisión

1. **Flujo y regla:** cada **asistencia** (evento `asistencia.registrada`) acredita
   **100 puntos en MELT** al email del alumno, además de los 500 del gimnasio. Las
   ausencias no se envían.
2. **Consumidor:** **benefits-service**, dueño de los puntos. En la **misma transacción**
   que el movimiento local guarda un envío en `envios_melt`, con la referencia
   `metalfitness-asistencia-<id de la asistencia>`.
3. **Entrega:** un proceso de Benefits revisa los envíos cada 5 s:
   - obtiene el email desde Members (`GET /internal/v1/alumnos/{id}/contacto`; no se
     guarda en benefits_db ni se registra en logs);
   - llama a MELT con un timeout de 40 s (arranque en frío de Render).
4. **Capa anticorrupción** (`internal/adapters/melt`): traduce la respuesta HTTP a
   resultados del dominio.

   | Respuesta | Resultado |
   |---|---|
   | `201`/`200` | Acreditado |
   | `202` | Pendiente de registro, con enlace |
   | `400`/`404`/`409`/`415`/`422` | Rechazado, sin reintento |
   | `401` | Reintento (es nuestra configuración) |
   | `5xx`, timeout o red | Reintento con espera de 10 s que se duplica hasta 10 min, siempre con la misma referencia |

   Un enlace vencido se renueva repitiendo la misma solicitud.
5. **Qué ve el usuario:** en su inicio, la tarjeta «Puntos MELT» con el estado de cada
   envío (acreditado, espera tu registro, enviando a MELT, no acreditado). El enlace de
   registro se muestra solo al destinatario autenticado y mientras está vigente.
6. **Disponibilidad:** MELT no forma parte del `/health/ready` de Benefits. Su caída
   solo demora los envíos.
7. **Configuración:** `BENEFITS_MELT_ENABLED`, `BENEFITS_MELT_URL`,
   `BENEFITS_MELT_PARTNER_KEY` (solo en `.env`, nunca en el repositorio) y
   `BENEFITS_MELT_TIMEOUT`.

## Consecuencias

- Positiva: la asistencia y el ledger local quedan aislados de MELT. Exactamente un
  envío por asistencia (restricción única en la base) y una referencia inmutable
  (trigger), así que nunca hay doble acreditación.
- Negativa: los puntos MELT llegan con demora. Si la persona no tiene cuenta MELT, queda
  pendiente hasta que se registre con el enlace.
- Riesgo: un cambio incompatible del contrato. Lo detecta el test de contrato sobre la
  copia versionada del OpenAPI.

## Cómo se verificó

- Test de contrato (`internal/adapters/melt/contract_test.go`) contra el OpenAPI de MELT.
- Tests del adaptador con un servidor simulado (cada código, timeout y red caída).
- Tests del dominio y del caso de uso.
- Integración con PostgreSQL real: transacción, reentrega sin duplicar, ausencias,
  inmutabilidad, enlace vencido.
- Prueba de punta a punta en el stack local contra el servidor real de MELT el
  11/11/2026: asistencia → 500 puntos locales y envío registrado; sin la clave, MELT
  respondió `401` y el envío quedó pendiente con reintento, sin afectar los puntos
  locales.
- Con la clave: MELT respondió `202` con enlace de registro y el alumno ve el botón para registrarse. Mientras el envío está pendiente de registro se repite la misma solicitud cada 2 minutos (idempotente) para detectar el registro (`200`).
