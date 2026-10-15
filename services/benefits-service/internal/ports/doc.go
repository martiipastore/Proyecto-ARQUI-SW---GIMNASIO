// Package ports define los puertos de salida de benefits-service: las interfaces
// que la aplicación necesita del exterior (ledger y cuentas, catálogo, partners
// e idempotencia, validación de alumnos en members-service, reloj). Los
// implementan los paquetes de internal/adapters.
//
// Solo depende de domain. Los puertos se agregan junto con los casos de uso.
package ports
