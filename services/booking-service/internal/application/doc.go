// Package application contiene los casos de uso de booking-service y sus puertos
// de entrada (las interfaces que exponen a los adaptadores de entrada: HTTP, AMQP).
//
// Depende solo de domain y de los puertos de salida de internal/ports; nunca de
// un adaptador concreto. Los casos de uso se agregan en la etapa de clases y reservas.
package application
