# Etapa 10 — Entrenamiento, nutrición y consultas

**Estado:** verificación técnica local cerrada el 10/11/2026. El E2E `-training`
aprobó **27 escenarios**: los 20 de identidad, membresías y Booking más 7 de esta
etapa. Evidencia en [training-e2e.json](../evidence/entrega-final/etapa10/training-e2e.json).
Falta la revisión de otro integrante. No se publicó en Vercel.

Historias: HU-17, HU-18, HU-23, HU-24, HU-25, HU-33 y HU-34, más el escenario E2E-08
del SPEC. Reglas: RN-28, RN-29, RN-30, RN-31 y RN-37; D-33 y ADR-007. Contrato:
[training.v1](../api/training.v1.md).

## 1. Qué se agregó

**training-service** (patrón Capas, MongoDB `training_db`), detrás de `TRAINING_ENABLED`:

| Quién | Qué puede hacer |
|---|---|
| Alumno | Ver su plan de entrenamiento y su plan alimenticio vigentes, ver sus mediciones ordenadas por fecha, enviar consultas a su nutricionista y ver las respuestas. |
| Profesor asignado | Ver el plan vigente y el historial del alumno. Crear un plan nuevo: el anterior pasa a FINALIZADO y queda en el historial. |
| Nutricionista asignado | Cargar planes alimenticios y registrar mediciones (nunca se editan ni se borran). Ver la bandeja de consultas de sus pacientes y responder cada una una sola vez. |

**members-service:** `GET /internal/v1/asignaciones/vigentes?alumno_id=` o
`?profesional_id=` lista las asignaciones vigentes. Training lo usa para saber si el
alumno tiene nutricionista (CA-33.2) y para armar la bandeja (CA-34.1).

**Gateway:** matriz por rol para `/training` y `/nutrition` (`trainingAllowed`). Cada
rol llega solo a sus rutas y el administrador no opera planes.

**Frontend:**
- **Mi plan** (`#/app/plan`) para el alumno.
- **Ver** en «Mis alumnos» / «Mis pacientes» abre la ficha `#/prof/alumnos/{id}` o
  `#/nutri/pacientes/{id}`. El formulario del plan de entrenamiento permite agregar días
  y ejercicios dinámicamente.
- **Consultas** (`#/nutri/consultas`) es la bandeja del nutricionista.

## 2. Autorización (RN-30, D-33, ADR-007)

1. **Gateway:** autorización gruesa por rol y ruta.
2. **Training, rol:** verifica el subtipo exacto. Un nutricionista no crea planes de
   entrenamiento aunque esté asignado (CA-23.5) y un profesor no carga planes
   alimenticios (CA-24.2).
3. **Training, asignación:** en **cada** operación sobre un alumno consulta a Members si
   la asignación está vigente, **sin caché**. Revocarla impide la siguiente operación,
   sin período de gracia.
4. **Members caído:** la operación falla cerrada con `503 DEPENDENCIA_NO_DISPONIBLE` y no
   modifica datos. Las consultas propias del alumno no dependen de Members.

La identidad llega solo del gateway con la clave interna (ADR-006, regla 10). Sin ella
training responde 401.

## 3. Garantías en la base (regla 8)

- **Un solo plan VIGENTE por alumno:** índice único parcial
  `{alumno_id} WHERE estado = "VIGENTE"` en planes de entrenamiento y alimenticios.
  Mongo corre standalone, sin transacciones. Crear un plan finaliza el vigente y después
  inserta el nuevo. Si dos altas compiten, el índice rechaza una y el repositorio la
  reintenta, así que siempre queda exactamente uno. Se probó con 10 altas concurrentes.
- **Mediciones válidas:** un validador `$jsonSchema` exige peso > 0 y porcentajes entre
  0 y 100 aunque se escriba directo en Mongo. El repositorio no ofrece modificar ni
  borrar mediciones (RN-31) y la API no tiene esas rutas.
- **Consultas:** validador de estado (`PENDIENTE` o `RESPONDIDA`). Responder es una sola
  actualización condicional `{_id, estado: PENDIENTE}`: entre respuestas simultáneas
  gana una sola (CA-34.4). Se probó con 8 concurrentes.
- `EnsureSchema` crea colecciones, validadores e índices al arrancar y es idempotente.
  Alcanza con el rol `readWrite` que ya tiene `training_user`.

## 4. Decisiones de implementación

- **Consultas sin nutricionista fijo:** una consulta PENDIENTE no queda atada a un
  nutricionista. La bandeja muestra las consultas de los **pacientes actuales**, así
  que si cambia la asignación las pendientes pasan al nuevo (RN-37) y el anterior deja
  de verlas. La respuesta registra quién respondió (CA-33.3).
- **Vigencia vencida:** un plan de entrenamiento con `vigencia_hasta` anterior a hoy se
  informa como FINALIZADO y el alumno ve «sin plan» (SPEC §7).
- **Fecha del plan alimenticio:** se fija con el día del alta («hoy» en hora de Argentina).
- **Contrato:** se agregó `GET /nutrition/plans?alumno_id=` para que el nutricionista
  vea el plan vigente y el historial, simétrico al de entrenamiento.

## 5. Verificación

```text
go run ./tests/e2e/identity/main.go -training
```

Levanta MongoDB 7 y training-service reales, más la regresión de identidad, membresías
y Booking. Escenarios:

- profesionales reales (nutricionistas y otro profesor dados de alta y activados por
  correo) con asignaciones en Members;
- plan vigente, historial y validaciones (CA-17.1/17.2, CA-23.1/23.2/23.3);
- autorización (CA-23.4/23.5, CA-24.2, alumno, anónimo), sin modificar datos ante rechazos;
- plan alimenticio y mediciones inmutables (CA-18.1/18.2, CA-24.1, CA-25.1 a 25.4);
- consultas (CA-33.1 a 33.4, CA-34.1 a 34.4, E2E-08);
- revocación inmediata, Members caído con 503 y recuperación;
- garantías de la base vistas con `mongosh` (validador e índice único).

Tests:

- `TestTrainingMongoReal` contra MongoDB 7: reemplazo con historial, índice único,
  10 altas concurrentes, validador, orden de mediciones y respuesta única concurrente;
- unitarios de la capa service: todas las reglas, autorización, revocación, Members
  caído y reasignación de nutricionista;
- handler: identidad del gateway, entrada estricta y catálogo de errores;
- gateway: matriz de permisos por rol.

Resultados del 10/11/2026: vet y tests de Members, Training y Gateway; `tsc` y build de
Vite aprobados; E2E de 27 escenarios aprobado en la primera corrida. No hay
automatización visual del navegador.

## 6. Límites

- Sin transacciones de Mongo: si el proceso se corta entre finalizar el plan vigente e
  insertar el nuevo, el alumno queda sin plan vigente hasta que el profesional lo vuelva
  a crear. Nunca quedan dos vigentes. Un replica set lo evitaría, pero cambia la
  infraestructura y queda para decidir con el equipo.
- No hay notificaciones de consulta respondida (no lo pide el SPEC).
- Training no se publicó en Vercel (ADR-021).
