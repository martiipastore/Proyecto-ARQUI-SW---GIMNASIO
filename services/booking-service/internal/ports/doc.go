// Package ports define los puertos de salida de booking-service: las interfaces
// que la aplicación necesita del exterior (repositorios, verificación de
// membresía en members-service, publicación de eventos por outbox, modelo de
// lectura, reloj). Los implementan los paquetes de internal/adapters.
//
// Solo depende de domain. Los puertos se agregan junto con los casos de uso.
package ports
