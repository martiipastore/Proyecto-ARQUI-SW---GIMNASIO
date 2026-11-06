# CB-02 — Integraciones y ledger: validación local

Actualización posterior: CB-03 también fue validada localmente; ver
[registro, activación y acceso](CB03_VALIDACION.md). Se conserva abajo la evidencia
de esta etapa sin atribuirle resultados de otras iteraciones.

Fecha: 06/11/2026. Implementación y verificación local completadas; revisión del
equipo según CONTRIBUTING pendiente antes de integrar. No es un despliegue ni
el cierre integral del Club. La siguiente etapa técnica es CB-03, incluida D-70.

## Cambios

- Migración aditiva 005: `partners.requiere_evento`. Conserva false en filas
  existentes y establece true como default para nuevas integraciones. El backend
  exige `eventId` en créditos si está activo; su omisión devuelve 400 sin crédito.
- Reutiliza permisos, ledger, bloqueo de presupuesto e identidad duradera de
  eventos existentes. No agrega tablas de saldo paralelas ni dependencias.
- Corrige fechas de replay recuperadas de PostgreSQL: UTC y microsegundos desde
  la primera respuesta de las acreditaciones nuevas. La prueba real inicial
  detectó distinta zona horaria; la regresión HTTP comprueba JSON idéntico.
- Docker obligatorio en tests de Benefits bajo CI o REQUIRE_DOCKER_TESTS=1;
  una falta del motor hace fallar la validación, en vez de aprobar omitiéndola.
- Contrato 1.4.0: política de evento por integración y ejemplos faltantes de
  registro/recibos. Los fixtures históricos declaran la excepción sin evento
  para conservar su propósito; no son plantillas de alta para partners nuevos.

## Evidencia

[Informe generado](evidence/club/cb02-tests.json): 53 resultados de tests/subtests
aprobados, cero fallidos y cero tests omitidos. Incluye contenedores PostgreSQL 16
aislados y elimina esos contenedores al finalizar. Los paquetes sin tests figuran
como skip a nivel paquete en el JSON de Go; no se cuentan como tests omitidos.
No se usaron DSN de desarrollo o producción ni datos personales reales.

| Comprobación | Resultado |
|---|---|
| Migraciones desde base vacía, repetición segura | Aprobado |
| Upgrade desde 001/002 con 300 puntos e historial previo | Aprobado; historial y saldo preservados |
| Crédito histórico sin eventId y nueva integración que lo exige | Aprobado |
| Ocho solicitudes simultáneas con mismo evento y distintas keys | Un movimiento, siete replays |
| Misma identidad de evento con otro usuario o contenido | Conflicto, sin crédito adicional |
| Evento repetido después de otra acreditación | Resultado original; saldo actual consultable por balance |
| Evento después de purgar respuesta HTTP vencida | Sin duplicación; identidad de evento inmutable |
| Primera respuesta con nanosegundos y replay por otra key | JSON idéntico con UTC y microsegundos |
| Presupuesto simultáneo entre cuentas y cambio de día argentino | Aprobado |
| Ocho solicitudes para cuota de tres | Tres admitidas, cinco limitadas dentro de la misma ventana |
| Credencial rotada, integración inactiva y permiso retirado | Acceso rechazado |
| HTTP 429 | RATE_LIMITED, Retry-After: 60, sin ejecutar consulta ni exponer key |
| Aislamiento, rollback, cancelación y contrato anterior | Regresión aprobada |
| go vet ./... en Benefits | Sin hallazgos |
| Spectral 6.14.3, fail-severity=warn, contrato 1.4.0 | Cero errores y advertencias |

## Reproducción

Con Docker disponible y Go instalado, ejecutar desde la raíz:

```powershell
& ([scriptblock]::Create((Get-Content scripts/verify-club-integrations.ps1 -Raw -Encoding UTF8)))
```

El script fuerza REQUIRE_DOCKER_TESTS=1 y ejecuta `go test ./... -count=1 -json`
en Benefits; genera un informe sin cuerpos, secretos ni logs de infraestructura.
El diagnóstico completo se conserva solo en la carpeta temporal local. Para vet,
usar `go vet ./...` desde services/benefits-service. Para contrato, `make contract-lint`
usa la imagen y las reglas ya previstas por el proyecto.

## Configuración y límites

Aplicar migraciones con el procedimiento existente antes del binario nuevo.
Esta ejecución solamente migró bases efímeras de tests. Revisar y coordinar
`requiere_evento=true` para cada partner anterior cuando su cliente ya envíe
eventos estables. Desactivar esa política conserva solo deduplicación por key:
no prometer protección ante otra key para un evento ausente.

La política se verifica también en replay; activarla puede rechazar replays
históricos que omitan evento, sin repetir ni perder su movimiento. Los rechazos
de autorización/cuota/política anteriores a tomar la key no reservan esa key.
No cambiar identifiers para resolver timeouts: recuperar estado o reintentar la
misma intención. El saldo de replay es histórico, no el saldo actual.

Panel, auditoría completa de cambios administrativos y reglas por acción siguen
en CB-05/07. SMTP, login/JWT/roles, los diez E2E integrales y capacidad del nuevo
flujo siguen pendientes. No se habilitaron flags de identidad ni se modificaron
producción, permisos de terceros, GitHub o Vercel. ADR-016/018 mantienen estado
propuesto para revisión; las pruebas no constituyen aprobación del equipo.
