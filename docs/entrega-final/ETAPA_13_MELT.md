# Etapa 13 — Integración externa con MELT

**Estado al 11/11/2026:** implementada y validada de punta a punta contra el servidor
real de MELT **con la clave de integración**: una asistencia generó el envío de 100
puntos y MELT respondió `202` (pendiente de registro, porque el alumno no tiene
cuenta en MELT); el alumno ve el botón **Registrarme en MELT**. Decisión:
[ADR-009 v2](../adr/ADR-009-v2-acreditacion-melt.md) y D-92.

## 1. Fuentes

| Fecha | Qué se recibió | Uso |
|---|---|---|
| 10/11/2026 | [Manual PDF](../contracts/providers/melt/Manual_integracion_MELT_Grupo_Gimnasio.pdf): catálogo, registro referido, login y puntos MELT | Histórico. No definía un flujo que afectara al gimnasio; queda reemplazado por la API del 11/11 |
| 11/11/2026 | API «Acreditación externa de puntos» **v1.1.0**: [OpenAPI](../contracts/providers/melt/acreditar-puntos-v1.openapi.yaml) (SHA-256 `521a79c7cecc25fd124389509513462e95ef467c251611569cba6c1a68179d0b`) y [guía de integración](../contracts/providers/melt/INTEGRACION-GIMNASIO.md), copiados de la rama `feature/api-acreditacion-puntos-externos` del repositorio de MELT | Contrato vigente de la integración y base del test de contrato |

Base: `https://melt-gateway-prueba.onrender.com/api/v1` (entorno académico de
integración, en Render gratuito). Primer acceso medido: **25 s** hasta responder.

## 2. Flujo de negocio (decidido por el equipo el 11/11/2026)

1. El alumno reserva y la recepción registra su ingreso por DNI.
2. Booking publica `asistencia.registrada`; Benefits acredita **500 puntos del
   gimnasio** y, **en la misma transacción**, registra un envío de **100 puntos a MELT**
   con la referencia `metalfitness-asistencia-<id de la asistencia>`.
3. Un proceso de Benefits (cada 5 s) obtiene el email del alumno desde Members y llama
   a `POST /beneficios/acreditaciones` con `X-Partner-Key`.
4. Según la respuesta de MELT:

   | Respuesta | Estado del envío | Qué ve el alumno |
   |---|---|---|
   | `201` / `200` | `ACREDITADO` | «Acreditados en MELT» |
   | `202` | `PENDIENTE_REGISTRO` | «Esperan tu registro» y el botón **Registrarme en MELT** con el enlace privado. Benefits repite la misma solicitud cada 2 minutos: MELT devuelve el mismo pendiente mientras el enlace esté vigente (verificado: mismo `venceEn`) y responde `200` cuando la persona se registra, así que la tarjeta pasa a «Acreditados». El enlace vence a los 7 días; vencido, se renueva repitiendo la misma solicitud |
   | `5xx`, timeout, red o `401` | `PENDIENTE_ENVIO` | «Enviando a MELT»; reintento a los 10 s, 20 s, 40 s… hasta 10 min, siempre con la misma referencia |
   | `400`, `404`, `409`, `415`, `422` | `RECHAZADO` | «No acreditados», con el motivo |

Las ausencias no se envían. La cantidad la decide el gimnasio; MELT no conoce
membresías ni asistencias.

## 3. Qué se implementó

| Componente | Cambio |
|---|---|
| Benefits — dominio | `PuntosMeltPorAsistencia = 100`; `EnvioMelt` con estados, espera creciente acotada y renovación del enlace vencido |
| Benefits — aplicación | `BookingEvents.ConEnviosMelt()` crea el envío junto con la asistencia; `EnviosMelt.Procesar` entrega los envíos vencidos; `MisEnvios` |
| Benefits — PostgreSQL | Migración `008_envios_melt.sql`: un envío por asistencia (`origen_id` único), referencia única, coherencia estado/enlace y trigger que impide cambiar la referencia, los puntos o el origen, y borrar envíos |
| Benefits — anticorrupción | `internal/adapters/melt`: cuerpo exacto del contrato y traducción de cada código a un resultado de dominio |
| Benefits — HTTP | `GET /api/v1/benefits/me/melt` para el alumno; el enlace solo se entrega vigente y a su destinatario |
| Members | `GET /internal/v1/alumnos/{id}/contacto`: email de un alumno activo, solo con la clave interna; el gateway no lo expone |
| Gateway | Ruta propia `/api/v1/benefits/me/melt` habilitada para cualquier rol autenticado (Benefits filtra por `X-User-Id`) |
| Frontend | Tarjeta «Puntos MELT» en el inicio del alumno |
| Configuración | `BENEFITS_MELT_ENABLED`, `BENEFITS_MELT_URL`, `BENEFITS_MELT_PARTNER_KEY` (solo `.env`), `BENEFITS_MELT_TIMEOUT` (40 s) |

**Seguridad:**
- La clave nunca se registra en logs ni se commitea.
- El email no se guarda en benefits_db ni aparece en logs.
- El enlace de registro no se registra en logs (lo pide MELT).

## 4. Pruebas

| Prueba | Resultado |
|---|---|
| **Test de contrato** (`adapters/melt/contract_test.go`) sobre el OpenAPI versionado de MELT: versión 1.x, operación, cabecera `X-Partner-Key`, URL base, nuestra solicitud dentro del esquema (`additionalProperties: false`, un solo destinatario y un solo identificador, longitudes), códigos interpretados y campos del `202` | Aprobado |
| Adaptador contra un servidor simulado: cuerpo y cabeceras, cada código, `202` sin enlace o con enlace no HTTPS, timeout y red caída | Aprobado |
| Dominio y caso de uso: 100 por asistencia, ausencias excluidas, referencia estable, espera creciente acotada, MELT caído, Members caído, alumno dado de baja | Aprobado |
| PostgreSQL real (testcontainers): envío en la misma transacción, reentrega del evento sin duplicar, ausencias, enlace vigente y vencido, inmutabilidad, flag apagado | Aprobado |
| Members: tests de servicio y handler | Aprobados |
| **Punta a punta contra el MELT real, sin clave** | Ingreso por DNI → saldo local de 500 a 1.000 → envío registrado → `401 PARTNER_NO_AUTORIZADO` → envío «Enviando a MELT» con reintento, sin afectar los puntos locales |
| **Punta a punta contra el MELT real, con la clave** (11/11, 13:57) | El primer reintento con clave falló de forma transitoria (MELT despertando) y se reprogramó. El siguiente obtuvo `202` con `registroUrl` HTTPS y vencimiento a 7 días → estado «Esperan tu registro» y botón **Registrarme en MELT** en la tarjeta del alumno. Repetir la solicitud con la misma referencia devolvió el mismo pendiente (idempotente) |

## 5. Pendiente

1. En la demostración: abrir el enlace desde la tarjeta, registrarse en MELT con
   `alumno@gym.local` y ver que, en un máximo de 2 minutos, la tarjeta pasa a
   «Acreditados en MELT» (`200`).
2. Antes de la demostración, «despertar» MELT (Render gratuito tarda ~25 s).
3. Jornada de fallas: mostrar MELT caído o lento sin efecto en la asistencia
   (POSTMORTEM, etapa 14).
4. Pedir al grupo de MELT que rote la clave después de la evaluación: se compartió por
   un canal no privado.
