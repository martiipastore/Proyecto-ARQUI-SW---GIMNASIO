package application

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/ports"
	"strings"
	"time"
)

type BookingEvents struct {
	ledger ports.BookingLedger
	clock  ports.Clock
	ids    ports.IDGenerator
}

func NewBookingEvents(l ports.BookingLedger, c ports.Clock, ids ports.IDGenerator) *BookingEvents {
	return &BookingEvents{l, c, ids}
}

func (b *BookingEvents) Aplicar(ctx context.Context, h domain.HechoBooking) error {
	if !h.Valido() {
		return domain.ErrOperacionInvalida
	}
	// UUID y RFC3339 tienen representaciones equivalentes; una reserialización
	// no debe crear otro efecto ni un falso conflicto de deduplicación.
	h.EventoID = strings.ToLower(h.EventoID)
	h.AlumnoID = strings.ToLower(h.AlumnoID)
	h.OrigenID = strings.ToLower(h.OrigenID)
	h.ReservaID = strings.ToLower(h.ReservaID)
	h.ClaseID = strings.ToLower(h.ClaseID)
	h.OcurridoEn = h.OcurridoEn.UTC()
	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join([]string{h.Tipo, h.AlumnoID, h.OrigenID, h.ReservaID, h.ClaseID, h.OcurridoEn.Format(time.RFC3339Nano)}, "|"))))
	return b.ledger.AplicarHecho(ctx, h, fingerprint, func(c domain.CuentaBeneficios) (domain.CuentaBeneficios, domain.MovimientoPuntos, error) {
		id, err := b.ids.NewID()
		if err != nil {
			return c, domain.MovimientoPuntos{}, err
		}
		return c.AplicarBooking(h, id, b.clock.Now())
	})
}

func (b *BookingEvents) Historial(ctx context.Context, user string, page, size int) (domain.PaginaMovimientos, error) {
	if !domain.UUIDValido(user) || page < 1 || page > 1000000 || size < 1 || size > 100 {
		return domain.PaginaMovimientos{}, domain.ErrOperacionInvalida
	}
	return b.ledger.HistorialUsuario(ctx, user, page, size)
}
