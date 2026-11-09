# API de entrenamiento y nutrición — v1 (diseño)

Servicio: training-service (Capas, MongoDB). Base: `<gateway>/api/v1`, con los prefijos
`/training` y `/nutrition` (ARCHITECTURE §5.2). Historias: HU-17, HU-18, HU-23 a HU-25,
HU-33 y HU-34. Reglas: RN-28 a RN-31, RN-37 y D-16. Convenciones comunes en
[README.md](README.md). Implementación: etapa 10 de la entrega final.

Cada operación de un profesional consulta a members si la asignación está vigente,
sin caché (`GET /internal/v1/asignaciones`, D-33). Si members no responde, la operación
falla cerrada con `503 DEPENDENCIA_NO_DISPONIBLE` y no modifica datos (ADR-007).

## Entrenamiento

| Método y ruta | Quién | Qué hace |
|---|---|---|
| GET /training/plans/me | ALUMNO | Plan vigente propio (HU-17); 404 si no tiene (CA-17.2) |
| GET /training/plans?alumno_id= | PROFESOR asignado | Plan vigente e historial del alumno |
| POST /training/plans | PROFESOR asignado | Crea un plan; el vigente anterior pasa a FINALIZADO (CA-23.1/2) |

Plan:

```json
{"alumno_id":"…","objetivo":"Hipertrofia","vigencia_desde":"2026-11-10","vigencia_hasta":"2027-01-10","observaciones":"…","dias":[{"nombre":"Día 1","ejercicios":[{"nombre":"Sentadilla","grupo_muscular":"Piernas","series":4,"repeticiones":10,"peso_sugerido":40,"descanso_segundos":90,"observaciones":""}]}]}
```

Validaciones (SPEC §9): objetivo obligatorio; desde ≤ hasta; al menos un día y un
ejercicio por día; series y repeticiones enteros > 0 (CA-23.3); peso y descanso ≥ 0.

## Nutrición

| Método y ruta | Quién | Qué hace |
|---|---|---|
| GET /nutrition/plans/me | ALUMNO | Plan alimenticio vigente (CA-18.1) |
| POST /nutrition/plans | NUTRICIONISTA asignado | Plan con al menos una comida; el anterior pasa a FINALIZADO (CA-24.1) |
| GET /nutrition/measurements/me | ALUMNO | Mediciones ordenadas por fecha (CA-18.2) |
| GET /nutrition/measurements?alumno_id= | NUTRICIONISTA asignado | Historial del paciente |
| POST /nutrition/measurements | NUTRICIONISTA asignado | Agrega una medición; nunca se edita ni se borra (CA-25.1/2) |
| GET /nutrition/questions/me | ALUMNO | Mis consultas con respuesta, quién respondió y fechas (CA-33.3) |
| POST /nutrition/questions | ALUMNO con nutricionista | Envía una consulta (CA-33.1) |
| GET /nutrition/questions | NUTRICIONISTA | Consultas de mis pacientes, de la más antigua a la más reciente (CA-34.1) |
| POST /nutrition/questions/{id}/answer | NUTRICIONISTA del paciente | Responde una sola vez (CA-34.2/4) |

Medición: `fecha` ≤ hoy, `peso` > 0, `% grasa` y `% muscular` entre 0 y 100 con suma
≤ 100, `masa_muscular` entre 0 y el peso (SPEC §9, CA-25.3/4).

## Errores propios

| HTTP | `code` | Cuándo |
|---|---|---|
| 403 | `SIN_PERMISO` | Alumno no asignado, o rol incorrecto (CA-23.4/5, CA-24.2, CA-34.3) |
| 422 | `SIN_NUTRICIONISTA` | El alumno no tiene nutricionista asignado (CA-33.2) |
| 409 | `TRANSICION_INVALIDA` | Responder una consulta ya respondida (CA-34.4) |
| 503 | `DEPENDENCIA_NO_DISPONIBLE` | Members no confirma la asignación |
