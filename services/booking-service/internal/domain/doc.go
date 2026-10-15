// Package domain es el núcleo de booking-service: entidades (Actividad, Horario,
// Clase, Reserva, Asistencia), value objects, reglas de negocio y errores de dominio.
//
// Reglas (ADR-002): solo depende de la biblioteca estándar; no importa frameworks,
// drivers ni otros paquetes del servicio; las structs no llevan tags (json, db,
// gorm, bson). Lo verifica architecture_test.go.
//
// Las entidades se agregan en la etapa de clases y reservas.
package domain
