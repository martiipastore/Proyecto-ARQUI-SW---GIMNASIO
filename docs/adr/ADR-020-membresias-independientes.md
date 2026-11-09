# ADR-020 — Membresías con historial independiente del ledger

| Campo | Valor |
|---|---|
| Estado | Propuesto para revisión; implementación local autorizada por CB-04 |
| Fecha | 2026-11-06 |
| Autoría | Borrador para el equipo METALFITNESS |
| Relacionado con | D-66/67; HU-CB-03; RN-CB-01/02/08/11; HU-03/04/05/12/27; RN-01–06; ADR-003/005 v2/006/017 |

## Contexto

Cuenta, puntos y membresía son independientes. Members ya tiene usuarios y acceso;
Benefits conserva su ledger. CB-04 agrega vigencia e historial sin llamadas que
borren o reinicien puntos. Se preservan no solapamiento, días inclusivos en
Argentina y renovación como nueva membresía, según SPEC y su adenda.

## Alternativas

- Guardar vigencia en la cuenta de puntos: mezcla propietarios e invita a borrar
  saldo al vencer. Descartado por D-66.
- Sobrescribir una membresía al renovar/cambiar tipo: pierde historial. Descartado
  por RN-06 y S-13.
- Nuevos registros en members_db, elegido para implementación local: conserva
  límites y patrón Capas, PostgreSQL e interfaces existentes.

## Decisión

Migración aditiva con cuatro tipos configurables, membresías, historial inmutable,
respuestas idempotentes y outbox de eventos en Members. Usar exclusión de rangos
de fecha por usuario para no superponer estados ACTIVA, con btree_gist de PostgreSQL
(ya previsto en ADR-003 para rangos; no agrega productos externos). Bloquear al
usuario al calcular renovaciones concurrentes. Expiración usa el día argentino;
las consultas calculan vigencia sin depender de la puntualidad del worker.

Asignar recibe fechas explícitas: no introduce una política arbitraria de fin de
mes. **Actualización del 09/11/2026 (D-77):** el equipo definió esa política. El
vencimiento se calcula según la duración del tipo (día anterior al mismo día del
período siguiente; último día del mes si ese día no existe) y, si se envía, debe
coincidir. Solo se asignan membresías a usuarios ALUMNO; la regla se comprueba en
`service`, no en el repositorio. Ver docs/api/memberships.v1.md. Renovar calcula inicio desde hoy o el día posterior al último período activo,
y recibe el vencimiento explícito. Tipo y cupo de clases se copian al otorgamiento:
editar configuración no cambia membresías anteriores. Los meses iniciales 1/3/12
proceden del SPEC; el tipo por clases no inventa cupo ni duración comerciales.
Cambio de tipo conserva la anterior mediante cancelación justificada y una nueva
asignación/renovación; no editar fechas ni tipo históricos.

Escrituras solo ADMINISTRADOR, con actor/justificación e Idempotency-Key. Lectura
propia por gateway y consulta administrativa por usuario. El servicio comprueba
permisos finos y nunca toma el actor del body. Partners no asignan membresías por
esta API; el otorgamiento promocional y sus reglas pertenecen a CB-05.

## Consecuencias

No hay dependencia desde membresías hacia saldo, ni conexión a benefits_db. El
estado vencido/cancelado no elimina cuenta, movimientos o configuración original.
Outbox se confirma en la misma transacción de asignación/renovación/cancelación;
su publicación al broker y consumidores de Booking/Notification siguen pendientes
de esas capacidades: no anunciar cancelación real de reservas ni email de membresía
entregado por escribir el evento. Bienvenida externa conserva su flujo CB-03.

No se implementan precios/cobros ni consumo de clases en reservas todavía. El
cupo queda registrado para su futura validación, sin simular asistencias.

## Verificación

PostgreSQL real: migraciones repetibles, exclusión concurrente, renovación, fin
de día, finalizados inmutables, evento/auditoría atómicos e idempotencia. HTTP por
gateway: alumno solo consulta sus datos y administrador gestiona. E2E-CB-03/04/09
deben conservar saldo e IDs de movimientos al vencer, volver y cambiar tipo.
Actualizar pruebas, contratos, decisiones y Word; revisión del equipo pendiente.

Teórico: propiedad/transacciones pp. 23/27–28, recuperación pp. 103–105/113–116,
capas pp. 118/120–124. SQL, estructuras e implementación son elecciones del
proyecto; no se atribuyen al apunte. Referencia complementaria:
https://www.postgresql.org/docs/16/btree-gist.html
