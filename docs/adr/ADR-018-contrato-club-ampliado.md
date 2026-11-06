# ADR-018 — Ampliar el contrato del Club conservando v1

| Campo | Valor |
|---|---|
| Estado | Propuesto; contrato 1.4.0 local, Spectral sin hallazgos y CB-02 validada; revisión y resto de flujos pendientes |
| Fecha | 2026-11-06 |
| Autores | Borrador preparado por el asistente para el equipo METALFITNESS; revisión de otro integrante pendiente |
| Decisión del TP | D8 |
| Relacionado con | D-63–69; HU-CB-01/04/07/08; RN-CB-03–06/12–15; ADR-008 v3/016/017 |
| Etapas | CB-01/02/03/08 |

## Contexto

El contrato 1.1.1 exponía crédito/saldo/historial reales y operaciones adicionales
en mock. Los requisitos revisados permiten alta externa y exigen permisos,
límites y un identificador de evento independiente de la key de solicitud.
La capacidad propia ofrecida a otra plataforma no resuelve D9 (consumir un proveedor).

## Alternativas consideradas

- Reemplazar v1 por proyecto/API nuevos: innecesario y contrario a conservar lo
  existente; obligaría a migrar clientes sin un cambio incompatible.
- Hacer eventId obligatorio en credits: rompe clientes previos que no lo enviaban.
- Extensión compatible de v1, elegida: eventId opcional en credits y rutas de
  registro con evento obligatorio; sin renombrar solicitudes/respuestas previas.

## Decisión

Registrar 1.2.0 (eventId/políticas) y 1.3.0 (external-register/consulta de recibo)
en OpenAPI y CHANGELOG. Mantener inglés/camelCase para partners y rutas
`/partner-api/v1`; APIs internas en snake_case. Registrar requiere permiso
separado y responde 202 con recibo persistente, sin revelar si existía el correo.
Consulta de estado/saldo exige autorización. El evento deduplica también entre
registro y credits. La cuota devuelve 429 RATE_LIMITED con Retry-After.

La revisión 1.4.0 documenta la política configurable requiere_evento (migración
005), conservando clientes existentes y exigiendo evento por defecto a nuevas
integraciones. Omitirlo cuando es obligatorio devuelve 400 INVALID_REQUEST.
Permisos, cuotas y políticas se comprueban también antes de devolver un replay.
Las acreditaciones nuevas usan UTC y precisión de microsegundos; no se modifican
respuestas históricas persistidas. Se completan los ejemplos de registro/recibo
exigidos por las reglas Spectral del proyecto.

No dar por implementados catálogo, débitos o canjes por aparecer en OpenAPI/mock.
Un cambio incompatible futuro requiere versionado según las decisiones previas.
ADR-008 v3 conserva updatedAt y su estado; este ADR no lo reemplaza antes de revisión.

## Consecuencias y fundamento

Clientes sin eventId conservan solo deduplicación por key. Las integraciones nuevas
deben enviar evento estable. Un 202 no confirma acreditación inmediata: consultar
accepted/completed y completar por reintentos. Flags, configuración y pruebas reales
son condiciones distintas de la versión del archivo; no activan producción.

BASE_TEORICA.md relaciona contratos/adaptadores/cuotas con pp. 138 y 142–148 e
idempotencia con pp. 103–105/113–115. Versiones, campos y estados son elecciones
del proyecto, no prescripciones del material de clase.

## Cómo se verificará

Spectral, referencias, ejemplos, compatibilidad sin eventId, 202/polling,
401/403/404/409/429/503 y errores de transporte. Probar que una integración no
consulta recibos/saldos ajenos y que repetir un evento no duplica puntos. En local
pasaron sintaxis/referencias YAML y tests de adaptadores. En CB-02 pasaron también
Spectral y PostgreSQL real (ver CB02_VALIDACION.md). CB-03 validó después registro,
autenticación JWT y SMTP real local (CB03_VALIDACION.md). Despliegue, proveedor
productivo de correo y operaciones restantes del Club siguen pendientes.
