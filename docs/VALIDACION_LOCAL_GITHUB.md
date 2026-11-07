# Verificación local frente al bloqueo de GitHub Actions

Fecha: 6 de noviembre de 2026. Base verificada: `ecd5ab5`, incorporada en
`origin/main` mediante PR #9. Esta ejecución no equivale a checks verdes de
GitHub, aprobación de ADR ni validación productiva.

GitHub rechazó iniciar los workflows `ci` y `contract` con el mensaje
«The job was not started because your account is locked due to a billing issue».
No se modificaron facturación, protecciones ni workflows para ocultar ese estado.

## Resultados locales

- Frontend: `npm.cmd run build` aprobado; TypeScript y Vite 6.4.3.
- Contrato: Spectral 6.14.3 aprobado, sin advertencias ni errores.
- Mock: Prism 5.15.10 aprobó los ocho ejemplos del workflow: catálogo, crédito,
  saldo, débito, saldo insuficiente, canje, movimientos y falta de autenticación.
  Esto valida ejemplos del contrato, no implementaciones pendientes de canjes.
- E2E: `go run ./tests/e2e/identity/main.go -memberships` aprobó 14 escenarios
  con servicios HTTP reales, PostgreSQL 16 y Mailpit aislados. Incluye alta y
  vinculación, activación de un solo uso, recuperación tras respuesta perdida,
  SMTP caído, autorización y conservación de 300 puntos e IDs de movimientos al
  vencer, renovar y cambiar de membresía. Evidencia:
  [cb04-e2e.json](evidence/club/cb04-e2e.json).
- Documentos: `scripts/sync-club-docs.ps1 -Check` confirmó los tres Word.
- Ejecutor E2E: `go vet` aprobado; se aplicó únicamente formato `gofmt`.
- Tests y `go vet` de los siete módulos: aprobados con Go 1.26.1 local, incluyendo
  integración con PostgreSQL y MongoDB reales. Se verificaron `pkg`, Gateway,
  Members, Booking, Benefits, Training y Notification. También aprobaron tests y
  `go vet` del inyector de fallas y `go vet` del diagnóstico de carga.
- Linter: el binario oficial golangci-lint 2.1.6 no es compatible con el Go
  1.26.1 instalado; se verificó con Go 1.22.12, sin cambiar `go.mod`.
  Staticcheck detectó QF1003 en `pkg/access/token.go`: se reemplazó la selección
  `if/else if` de formato PEM por `switch`, sin alterar formatos admitidos ni
  validaciones. Los tests y `go vet` de autenticación volvieron a aprobar.
  Members tuvo tres errores errcheck en cierres diferidos de HTTP/SMTP y QF1001
  en la validación de la URL pública. Se explicitó el descarte de errores de
  limpieza y se separaron las condiciones equivalentes: HTTPS o HTTP solo en
  loopback. No se modificaron las restricciones de seguridad. Los tests y vet
  de Members se repitieron después de corregirlos y aprobaron. Benefits tuvo
  otro cierre HTTP señalado por errcheck en el adaptador de Members; corregido
  con el mismo criterio, sus tests y vet aprobaron. Gateway, Booking, Training,
  Notification, `pkg`, Members y Benefits aprobaron el linter final sin hallazgos.
  La carga inicial excedió cinco minutos en algunos intentos; Benefits se repitió
  con un límite de diez minutos y terminó correctamente. No se deshabilitaron
  linters ni se modificó la configuración de CI.

Los tests generales se ejecutan con `-count=1`, `REQUIRE_DOCKER_TESTS=1` y
contenedores de integración reales. El E2E y Prism usan recursos temporales;
no se utilizó `.env`, no se modificó producción ni se borraron volúmenes existentes.
El E2E se ejecutó antes de las correcciones de lint; después de ellas se repitieron
los tests y vet de autenticación, Members y el adaptador modificado de Benefits.

Esta verificación no completa por sí sola la documentación ni la aceptación de
CB-04, y no implementa las etapas pendientes CB-05 a CB-08. El cierre documental
de membresías, la revisión del equipo y la configuración productiva conservan sus
pendientes. No hay automatización de navegador en esta ejecución.
