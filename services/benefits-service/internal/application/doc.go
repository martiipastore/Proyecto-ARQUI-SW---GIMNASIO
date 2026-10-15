// Package application contiene los casos de uso de benefits-service y sus puertos
// de entrada (las interfaces que usan los adaptadores HTTP interno, HTTP de
// partners y AMQP).
//
// Depende solo de domain y de los puertos de salida de internal/ports; nunca de
// un adaptador concreto. Los casos de uso se agregan en la etapa del Club de Beneficios.
package application
