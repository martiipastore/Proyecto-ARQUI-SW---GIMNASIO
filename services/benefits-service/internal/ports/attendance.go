package ports

import (
	"context"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
)

type BookingLedger interface {
	AplicarHecho(context.Context, domain.HechoBooking, string, func(domain.CuentaBeneficios) (domain.CuentaBeneficios, domain.MovimientoPuntos, error)) error
	HistorialUsuario(context.Context, string, int, int) (domain.PaginaMovimientos, error)
}
