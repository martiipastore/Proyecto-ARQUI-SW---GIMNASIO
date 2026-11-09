# API del Club de Beneficios para la aplicación — v1 (diseño)

Servicio: benefits-service (Hexagonal). Base: `<gateway>/api/v1`. Es la API de la
aplicación del gimnasio; la de otros grupos es la [API de partners v2](../contracts/README.md).
Historias: HU-10, HU-11, HU-19, HU-35 y HU-CB-07. Catálogo, canjes y débitos quedan
fuera por D-76. Convenciones comunes en [README.md](README.md).

## Endpoints

| Método y ruta | Quién | Qué hace | Estado |
|---|---|---|---|
| GET /benefits/me | Cualquier rol con cuenta | Saldo propio: `{"balance":…, "updatedAt":…}` | Implementado (CB-03) |
| GET /benefits/me/movements | Cualquier rol con cuenta | Movimientos propios paginados (HU-19) | Etapa 6 |
| GET /benefits/accounts/{usuarioId} | ADMINISTRADOR | Saldo y movimientos de una persona (HU-CB-07, HU-35) | Etapa 7 |
| POST /benefits/movements/{id}/revert | ADMINISTRADOR | Revierte con `motivo` e `Idempotency-Key` (HU-11) | Etapa 7 |
| GET /benefits/partners | ADMINISTRADOR | Integraciones con estado, permisos y límites | Etapa 7 |
| POST /benefits/partners | ADMINISTRADOR | Alta de integración; devuelve la API key **una sola vez** (CA-10.1) | Etapa 7 |
| POST /benefits/partners/{id}/rotate-key | ADMINISTRADOR | Nueva API key; la anterior deja de funcionar | Etapa 7 |
| PUT /benefits/partners/{id} | ADMINISTRADOR | `activo`, permisos y límites (ADR-016) | Etapa 7 |

Hoy el alta y la rotación de integraciones se hacen con
`scripts/provision-club-partner.ps1` ([VERCEL_CLUB.md](../deploy/VERCEL_CLUB.md)).
La respuesta `/benefits/me` conserva sus claves actuales (`balance`, `updatedAt`) porque
ya la usa el frontend.

## Movimiento

```json
{"id":"…","fecha":"2026-11-10T13:05:00Z","motivo":"Asistencia a Funcional","puntos":500,"estado":"CONFIRMADO","origen":"ASISTENCIA","secuencia":12}
```

`origen`: `ASISTENCIA`, `INASISTENCIA`, `PARTNER` o `REVERSION`. Las penalizaciones se
muestran con signo negativo, incluida la de 0 puntos con saldo 0 (CA-22.11). El saldo
mostrado es la suma de todos los movimientos (CA-19.2).

## Errores propios

| HTTP | `code` | Cuándo |
|---|---|---|
| 404 | `CUENTA_NO_ENCONTRADA` | La cuenta todavía no existe (alta reciente, S-10) |
| 422 | `SALDO_INSUFICIENTE` | Revertir dejaría saldo negativo (CA-11.3) |
| 409 | `TRANSICION_INVALIDA` | Revertir un movimiento ya revertido (CA-11.2) |
