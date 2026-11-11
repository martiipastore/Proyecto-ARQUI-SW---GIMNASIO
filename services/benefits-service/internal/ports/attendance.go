package ports

import (
	"context"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
)

type BookingLedger interface {
	// AplicarHecho confirma el movimiento local y, si envio no es nil, registra
	// el envío al proveedor en la misma transacción (etapa 13): nunca se pierde
	// ni se duplica respecto de la asistencia.
	AplicarHecho(context.Context, domain.HechoBooking, string, func(domain.CuentaBeneficios) (domain.CuentaBeneficios, domain.MovimientoPuntos, error), *domain.EnvioMelt) error
	HistorialUsuario(context.Context, string, int, int) (domain.PaginaMovimientos, error)
}
