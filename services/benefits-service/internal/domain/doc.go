// Package domain es el núcleo de benefits-service: entidades (CuentaBeneficios,
// MovimientoPuntos, Beneficio, Canje, Partner), value objects, reglas del ledger
// y errores de dominio.
//
// Reglas (ADR-002): solo depende de la biblioteca estándar; no importa frameworks,
// drivers ni otros paquetes del servicio; las structs no llevan tags (json, db,
// gorm, bson). Lo verifica architecture_test.go.
//
// Las entidades se agregan en la etapa del Club de Beneficios.
package domain
