package domain

import "errors"

// Errores de negocio de booking. El adaptador HTTP los traduce a los códigos del
// catálogo de ARCHITECTURE §12.1.
var (
	ErrValidacion              = errors.New("datos inválidos")
	ErrNoEncontrado            = errors.New("recurso inexistente")
	ErrSinPermiso              = errors.New("sin permiso sobre el recurso")
	ErrSinCupo                 = errors.New("sin cupo")
	ErrReservaDuplicada        = errors.New("reserva duplicada")
	ErrTurnoMusculacionDiario  = errors.New("ya tiene un turno de Musculación ese día")
	ErrSuperposicionReserva    = errors.New("superposición con otra reserva")
	ErrClaseNoFutura           = errors.New("clase no futura")
	ErrClaseCancelada          = errors.New("clase cancelada")
	ErrPlazoCancelacionVencido = errors.New("plazo de cancelación vencido")
	ErrTransicionInvalida      = errors.New("transición inválida")
	ErrSuperposicionProfesor   = errors.New("el profesor ya tiene una clase superpuesta")
	ErrMembresiaNoVigente      = errors.New("membresía no vigente")
	ErrUsuarioInactivo         = errors.New("usuario inactivo")
	ErrSinReservaParaHorario   = errors.New("sin reserva para este horario")
	ErrConflictoIdempotencia   = errors.New("la clave ya identifica otra operación")
	ErrOperacionEnCurso        = errors.New("operación en curso con la misma clave")
)
