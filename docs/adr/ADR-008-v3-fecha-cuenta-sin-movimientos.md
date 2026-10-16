# ADR-008 v3 — Definir updatedAt para cuentas sin movimientos

| Campo | Valor |
|---|---|
| **Estado** | Aceptado el 16/10/2026 |
| **Fecha** | 2026-10-16 |
| **Decisión del TP** | D8 |
| **Reemplaza** | [ADR-008 v2](ADR-008-v2-errores-gateway-partners.md) |
| **Relacionado con** | HU-32; RN-22; D-53 del registro |

## Contexto y alternativas

Balance.updatedAt es obligatorio y se describía como fecha del último movimiento, pero no estaba definido para una cuenta con saldo inicial cero y sin movimientos. Omitirlo o devolver null contradice el esquema; usar la hora de cada consulta inventaría una actualización de negocio que no ocurrió.

## Decisión

El equipo aprobó devolver la fecha de creación de la cuenta hasta el primer movimiento; después, la fecha de su último movimiento aplicado. Mantener todas las demás decisiones de ADR-008 v2 y publicar la aclaración descriptiva como OpenAPI 1.1.1, sin cambios de rutas ni esquemas de solicitud/respuesta.

La migración 002 conserva la fecha de creación de las cuentas nuevas. Para cuentas anteriores que no tuvieran ese dato, el valor inicial será la fecha de la migración; no se afirma recuperar una fecha histórica desconocida. En el desarrollo actual no existía API funcional que hubiera informado ese campo.

## Fundamento y consecuencias

El concepto de estado persistido e invariantes pertenece al modelo de dominio y su consistencia (teórico pp. 27–28 y 120); la semántica concreta de updatedAt es una decisión del contrato del proyecto, no una regla del apunte. Se evita presentar la hora de consulta como un cambio de saldo y se obtiene un valor estable también para cuentas vacías.

## Cómo se verifica

Consultar una cuenta vacía devuelve saldo 0 y fecha válida de creación; dos lecturas sin cambios conservan esa fecha. Tras acreditar, updatedAt corresponde al movimiento persistido. Validar las respuestas capturadas con Spectral y los esquemas del contrato.
