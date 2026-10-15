# Registro de decisiones

METALFITNESS — Práctico Integrador · Arquitectura de Software 2026 (UCC)

> Copia en texto de `Registro_de_Decisiones_Gimnasio.docx`. Es la versión que lee la IA; si se modifica el Word, actualizá también este archivo.

## Cómo usar este registro

Acá quedan las decisiones de negocio y de alcance que va tomando el grupo. Son la referencia para el SPEC.md, para los prompts que se le pasan a la IA y para la defensa. En el repo existe una copia en texto, docs/DECISIONES.md, que es la que lee la IA.

- Cada decisión tiene un identificador (D-NN) que no se reutiliza.
- Si una decisión cambia, no se borra: se marca como reemplazada en "Decisiones reemplazadas" y la nueva indica a cuál reemplaza.
- Las decisiones de arquitectura (D1 a D13 del enunciado) van en los ADR del repo (docs/adr/). Este registro es para las reglas del dominio y las decisiones de alcance.

## Decisiones confirmadas

| ID | Tema | Decisión |
|---|---|---|
| D-01 | Turnos de Musculación | De 08:00 a 23:00, con 7 turnos reservables de 2 horas por día: 08:00–10:00, 10:00–12:00, 12:00–14:00, 14:00–16:00, 16:00–18:00, 18:00–20:00 y 20:00–22:00. La última hora de apertura no genera un turno. Máximo 50 alumnos por turno. |
| D-02 | Cancelación de reservas | Hasta 1 hora antes del inicio. Los únicos límites de reservas son D-17 y D-18. |
| D-03 | Membresía para reservar | Tiene que estar vigente al reservar y en la fecha de la clase. |
| D-05 | Reservas sin asistencia | Al finalizar la clase, las reservas confirmadas sin ingreso válido (ver D-37) pasan automáticamente a AUSENTE y generan una penalización de hasta -100 puntos, sin saldo negativo. No existe regularización manual. |
| D-06 | Usuarios de los partners | Solo alumnos del gimnasio vinculados al partner. |
| D-07 | Qué ve el partner | El saldo total y solo los movimientos que hizo él. |
| D-08 | Límites de los partners | Sin tope de puntos. Los identificadores de operación se guardan al menos 30 días. |
| D-09 | Alumnos que ve cada profesional | El profesor ve a sus asignados y a los que reservaron en sus clases; los planes de entrenamiento, solo de sus asignados. La nutricionista ve solo a sus pacientes. |
| D-10 | Nutricionista | No dicta clases ni arma planes de entrenamiento. Carga planes alimenticios, registra mediciones y responde consultas de sus pacientes. |
| D-11 | Avisos al alumno | No se le avisa cuando se cancela una clase o su membresía. |
| D-12 | Membresías con fecha pasada | No se permiten: la fecha de inicio tiene que ser hoy o posterior. |
| D-13 | Puntos y beneficios | Los puntos no vencen. Los beneficios no tienen stock: siempre hay, van cambiando y los cambia el administrador. |
| D-14 | Puntos por actividad | Se asignan 500 puntos por cada clase asistida, sin diferenciar la actividad. La inasistencia a una reserva confirmada descuenta hasta 100 puntos, sin saldo negativo. |
| D-15 | Administrador y reservas | Puede cancelar una reserva en nombre de un alumno, pero no reservar por él. |
| D-16 | Consultas a la nutricionista | Una pregunta con una sola respuesta. Para repreguntar, el alumno manda una consulta nueva. Sin adjuntos ni chat en vivo. |
| D-17 | Musculación por día | Máximo 1 turno de Musculación por alumno por día. Si lo cancela, puede reservar otro turno del mismo día. |
| D-18 | Reservas que se pisan | No se permiten, ni entre actividades distintas ni con los turnos de Musculación. Una clase que termina a las 20:00 no se pisa con una que empieza a las 20:00. |
| D-19 | Tipos de membresía iniciales | Mensual (1 mes), Trimestral (3 meses) y Anual (12 meses). |
| D-20 | Cancelación de reservas por membresía o baja (arquitectura) | members-service publica los eventos membresia.cancelada y usuario.desactivado; booking-service los consume y cancela las reservas futuras del alumno (RN-04, RN-34). Ver docs/ARCHITECTURE.md §6–7 y ADR-005. |
| D-22 | Nombres de alumnos en los listados (arquitectura) | booking-service le pide a members-service los nombres de los alumnos de una clase. Si members-service no responde, el listado se muestra con los IDs y sin nombres (HU-21). Ver ADR-005. |
| D-24 | Retención de claves de idempotencia API de partners | 30 días (no 24 h), por partner, como fija la RN-27 y D-08. Se guardan también las respuestas de error. Ver docs/contracts/README.md y ADR-008. |
| D-25 | Códigos de error adicionales en la API de partners | Además de los pedidos se agregan BENEFIT_NOT_FOUND (404), BENEFIT_NOT_AVAILABLE (422) e IDEMPOTENCY_IN_PROGRESS (409, reintentable con la misma key). |
| D-26 | Máximo de puntos por operación | De 1 a 1.000.000 puntos por operación. Es un límite técnico de validación del contrato, no un tope de negocio (D-08 sigue vigente). |
| D-27 | Ruta pública de la API de partners | En producción: https://<host>/partner-api/v1/... a través del api-gateway. En el mock local: http://localhost:4010/v1/... Contrato en inglés y camelCase (excepción a la convención interna). |
| D-28 | Vinculación de usuarios de partners | Solo el administrador del gimnasio vincula cada alumno con el externalUserId del partner (HU-10). El partner no puede vincular por la API; si opera sobre un usuario no vinculado recibe 404 ACCOUNT_NOT_FOUND. |
| D-29 | Rutas públicas del api-gateway (arquitectura) | Los prefijos públicos van en inglés: /api/v1/auth, users, memberships, activities, classes, bookings, check-ins, benefits, training y nutrition, más /partner-api/v1. El dominio, el código y los campos JSON siguen en español y snake_case. Ver docs/ARCHITECTURE.md §5.2. |
| D-30 | Estructura de los servicios hexagonales (arquitectura) | booking-service y benefits-service usan internal/domain (solo biblioteca estándar, sin tags), internal/application (casos de uso y puertos de entrada), internal/ports (puertos de salida) e internal/adapters (http, postgres...). Lo verifican tests de arquitectura. Ver ADR-002. |
| D-31 | Puntos y penalización por asistencia | Cada ingreso válido registra la asistencia y suma 500 puntos. Si una reserva confirmada termina sin ingreso válido, pasa a AUSENTE y se descuentan hasta 100 puntos; el saldo nunca queda por debajo de 0. La penalización se aplica una sola vez mediante el evento inasistencia.registrada. |
| D-32 | Avisos de vencimiento de membresía | METALFITNESS envía al alumno un recordatorio 10 días calendario antes del vencimiento y una advertencia durante el día del vencimiento. Los envíos son asíncronos, idempotentes por membresía y tipo, y no dependen del partner externo. |
| D-33 | Asignaciones en training-service sin caché (arquitectura) | Reemplaza D-21. training-service consulta a members-service la asignación vigente antes de cada operación protegida. La autorización no se cachea; si members-service no puede confirmarla, la operación falla cerrada sin modificar datos (RN-30). Ver ADR-007. |
| D-34 | Borde público del despliegue (arquitectura) | Reemplaza D-23. El stack se despliega en una VM con Docker Compose. Traefik es la única entrada pública HTTPS: sirve la SPA y enruta las APIs al api-gateway. Los servicios y la infraestructura quedan en una red privada. El proveedor de nube se fija en D-41. Ver docs/ARCHITECTURE.md §10 y §14.2, y ADR-006. |
| D-35 | Horarios de las clases grupales | Musculación es la única actividad con turnos todo el día (D-01). Las grupales son de 60 minutos, en horarios fijos: GAP martes y jueves 09:00–10:00 y 17:00–18:00; Funcional lunes y miércoles 09:00–10:00 y 15:00–16:00; Strong Nation martes y sábados 09:00–10:00, martes 15:00–16:00 y sábados 12:00–13:00; Zumba lunes y miércoles 10:00–11:00 y 17:00–18:00. Las actividades son 5: Musculación, Funcional, GAP, Strong Nation y Zumba. |
| D-36 | Ingreso al gimnasio por teclado (kiosco) | Reemplaza D-04. En la entrada hay un teclado donde el alumno escribe solo su DNI (sin PIN). Si el DNI es correcto aparece su nombre completo y el estado de su membresía. Membresía vigente y reserva válida (D-37): nombre en verde y "Bienvenido". Membresía vencida: nombre en rojo y la fecha en que venció. Membresía cancelada, sin membresía, usuario inactivo o sin reserva para ese horario: nombre en rojo y el motivo. En rojo no puede pasar. El profesor no registra asistencia manualmente. |
| D-37 | Ventana de asistencia | El ingreso cuenta como asistencia si se hace desde 15 minutos antes del inicio de la clase hasta 30 minutos antes de su fin (clase de 1 h: de inicio −15 min a inicio +30 min; turno de Musculación de 2 h: de inicio −15 min a inicio +90 min). Ingreso dentro de la ventana: la reserva pasa a ASISTIDA y suma 500 puntos. Si la ventana se cierra sin ingreso, la reserva pasa a AUSENTE al finalizar la clase y se aplica la penalización (D-05). Ejemplo: reservas en Musculación 08–10 y Zumba 10–11; si llega a las 09:50, Musculación queda AUSENTE (−100) y Zumba ASISTIDA (+500). |
| D-38 | Reingreso | Los puntos van atados a cada reserva y nunca se suman dos veces por la misma. Si el alumno ya tiene la asistencia registrada y vuelve a ingresar el DNI antes de que termine esa clase, pasa en verde con "Bienvenido de nuevo" y sin puntos. Un ingreso posterior para otra reserva suma sus propios 500 puntos. |
| D-39 | Ingreso sin reserva | Si la membresía está vigente pero no tiene una reserva con la ventana de asistencia abierta (ni una asistencia en curso, D-38), no puede pasar: nombre en rojo y el motivo "sin reserva para este horario". |
| D-40 | Desafíos bonus | Se anuncian en la Entrega 2: tests e2e con Playwright de los flujos del SPEC §11 y actualización de la ocupación de las clases en tiempo real (Server-Sent Events). |
| D-41 | Proveedor de nube | Oracle Cloud Free Tier: una VM ARM "Always Free" con Docker Compose (completa D-34). |

## Decisiones reemplazadas

| ID | Tema | Decisión |
|---|---|---|
| D-04 | Registro de asistencia (reemplazada) | Reemplazada por D-36 y D-37 el 08/10/2026. Decisión anterior: el alumno ingresaba su DNI durante la clase, el sistema validaba membresía y reserva, registraba la asistencia y sumaba 500 puntos. |
| D-21 | Asignaciones en training-service (reemplazada) | Reemplazada por D-33 el 08/10/2026. Decisión anterior: training-service consultaba a members-service antes de cada operación y cacheaba la asignación durante 60 segundos. |
| D-23 | Despliegue en la nube (reemplazada) | Reemplazada por D-34 el 08/10/2026. Decisión anterior: el stack se desplegaba en una VM con Docker Compose y se describía al api-gateway como único componente expuesto por HTTPS. |

## Historial de cambios

| Fecha | Cambio |
|---|---|
| 07/10/2026 | Creación del registro con las decisiones D-01 a D-19. |
| 07/10/2026 | Se agregan D-20 a D-23: decisiones de arquitectura aprobadas por el grupo en la etapa 02 (eventos de cancelación, asignaciones, nombres de alumnos y despliegue en la nube). |
| 07/10/2026 | Se agregan D-24 a D-28: decisiones del contrato público de la API de partners (etapa 03). |
| 07/10/2026 | Se agregan D-29 y D-30: rutas del gateway en inglés y estructura de los servicios hexagonales (etapa de esqueleto). |
| 07/10/2026 | Se actualizan D-02, D-04, D-05 y D-14 (ingreso con DNI, cancelación hasta 1 hora antes, penalización por inasistencia). Se agregan D-31 y D-32. |
| 08/10/2026 | D-33 reemplaza D-21 y D-34 reemplaza D-23. |
| 08/10/2026 | Se agregan D-35 a D-41: horarios fijos de las grupales, ingreso por teclado en la entrada, ventana de asistencia, reingreso, ingreso sin reserva, bonus y proveedor de nube. D-36 y D-37 reemplazan D-04. Se ajusta D-05 para referirse a la ventana de D-37. |
