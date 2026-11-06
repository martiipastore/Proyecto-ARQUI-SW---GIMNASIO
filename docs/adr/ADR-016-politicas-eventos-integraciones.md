# ADR-016 — Políticas por integración y eventos externos

| Campo | Valor |
|---|---|
| Estado | Propuesto en sus detalles técnicos; controles implementados y validados localmente en CB-02; revisión del equipo y despliegue pendientes |
| Fecha | 2026-11-06 |
| Autores | Borrador preparado por el asistente para el equipo METALFITNESS; revisión de otro integrante pendiente |
| Decisiones del TP | D3, D7, D8, D10; conserva límites y patrones previos |
| Relacionado con | D-64/66; HU-CB-04/07; RN-CB-01–07; ADR-003/004 v2/007/015/018 |
| Etapa | CB-02; requisitos autorizados por el equipo, validación técnica/operativa pendiente |

## Contexto

La API existente tiene claves distintas por partner y vinculación por cuenta,
pero concede todos los permisos a un partner activo, admite el máximo técnico
fijo y deduplica solamente por Idempotency-Key durante su retención.

## Decisión

Mantener Hexagonal, PostgreSQL y los límites de servicio existentes. Extender
partners con permisos de lectura/escritura y máximos de puntos configurables.
La política de acreditación pertenece al dominio. PostgreSQL entrega los hechos
y serializa el consumo diario mediante bloqueo transaccional por integración.
No se agregan productos ni dependencias.

Agregar `eventId` opcional al contrato v1 para preservar clientes existentes.
La migración 005 agrega `requiere_evento`: false para integraciones existentes,
true por defecto para altas nuevas. El backend rechaza créditos sin evento cuando
esa política está activa. Es una concreción local D-73 de la regla ya solicitada;
una excepción de compatibilidad exige configuración explícita. La gestión mediante
panel sigue en CB-07. La política se verifica también en replay, sin mover saldo.
La identidad `(partner_id, evento_id)` y el movimiento se guardan en una sola
transacción. La identidad no se elimina al vencer la retención de respuestas.
El replay verifica permisos y vinculación vigentes. El saldo original del evento
permanece estable aunque la cuenta reciba otras acreditaciones.

Los máximos comerciales no se inventan: se conserva el máximo técnico y el
presupuesto diario queda sin configurar hasta que lo defina el gimnasio.
Esto no equivale a un control de tasa de solicitudes.

## Cuotas de solicitudes

La revisión posterior agrega en migración 004 una cuota de solicitudes autenticadas
por integración y minuto. Su valor inicial 120 es un parámetro operativo propio y
configurable, no un requisito del teórico ni una cuota aprobada para un partner real.

## Alternativas consideradas

- Confiar en el partner o compartir una clave: no satisface permisos, revocación
  individual ni límites internos de D-64.
- Deduplicar solo por key: permite duplicar el hecho con otra key o después de
  purgar la respuesta.
- Estado persistente y transacción PostgreSQL, elegida localmente: reutiliza el
  ledger con identidad duradera del evento y presupuesto concurrente; su costo
  es la contención de acreditaciones de una misma integración.

## Fundamento y parámetros

Según la revisión existente en BASE_TEORICA.md: transacciones e idempotencia
(pp. 27–28, 103–105 y 113–115), agregado/límite de consistencia (p. 120) y cuotas
de integración (pp. 138 y 142–148). Índices, funciones advisory y cifras son
elecciones del proyecto; no se atribuyen al PDF ni se dan por validadas.

## Consecuencias operativas y verificación

Las acreditaciones nuevas de una misma integración se serializan al comprobar
su presupuesto; es un costo explícito para evitar excederlo concurrentemente.
Las demás integraciones pueden operar en paralelo salvo que compartan cuenta.
La migración debe preceder al binario. Registro y acceso tienen implementación
local posterior en ADR-017; membresías y panel siguen pendientes según el
[plan CB](../PLAN_CLUB_BENEFICIOS.md). Se conserva el ledger existente.

Se verificarán permisos, límites, presupuesto concurrente entre cuentas, evento
repetido con otra key, revocación y rollback en PostgreSQL real. Probar migraciones
003/004/005 desde base nueva y migrada y compatibilidad sin eventId. Criterio: cero
duplicaciones, sobregiros o pérdida de historial. Tests/vet locales pasan; los de
base se omitían por falta de Docker en la iteración anterior.

Actualización del 06/11/2026: CB-02 ejecutada con PostgreSQL real; 53 resultados
test/subtest aprobados sin omisiones, vet y Spectral correctos. Se comprobó
concurrencia, presupuesto entre cuentas, cuota atómica, revocación, upgrade con
historial y retención duradera del evento. Se corrigió la representación de fechas
del replay: UTC y microsegundos desde la primera acreditación. Ver
[evidencia y límites](../CB02_VALIDACION.md). No acredita CB-03, SMTP o producción.

## Riesgos a revisar

| Riesgo | Impacto | Mitigación / aceptación pendiente |
|---|---|---|
| Contención del presupuesto por integración | Mayor latencia entre acreditaciones | Prueba de concurrencia/carga del código nuevo, sin reciclar métricas anteriores |
| Créditos de clientes previos sin eventId | La key nueva no identifica el mismo hecho | Compatibilidad preservada y flag requiere_evento implementado/probado; coordinar activación para clientes previos, sin afirmar deduplicación de evento ausente |
