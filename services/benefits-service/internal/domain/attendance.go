package domain

import (
	"math"
	"strings"
	"time"
)

const Asistencia = "ASISTENCIA"
const PenalizacionInasistencia = "PENALIZACION_INASISTENCIA"

// HechoBooking es el hecho de negocio traducido desde el adaptador AMQP.
// No conoce el envelope, JSON ni la infraestructura (RN-20/38/42).
type HechoBooking struct {
	EventoID, Tipo, AlumnoID, OrigenID, ReservaID, ClaseID string
	OcurridoEn                                             time.Time
}

func (h HechoBooking) Valido() bool {
	for _, id := range []string{h.EventoID, h.AlumnoID, h.OrigenID, h.ReservaID, h.ClaseID} {
		if !UUIDValido(id) {
			return false
		}
	}
	return (h.Tipo == Asistencia || h.Tipo == PenalizacionInasistencia) && !h.OcurridoEn.IsZero()
}

func UUIDValido(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

// AplicarBooking fija importes desde las reglas locales; no acepta un importe
// arbitrario del broker. El orden aplicado es la secuencia bajo bloqueo.
func (c CuentaBeneficios) AplicarBooking(h HechoBooking, id string, ahora time.Time) (CuentaBeneficios, MovimientoPuntos, error) {
	if !h.Valido() || c.secuencia == math.MaxInt64 {
		return c, MovimientoPuntos{}, ErrOperacionInvalida
	}
	puntos := int64(500)
	motivo := "Asistencia a clase"
	if h.Tipo == PenalizacionInasistencia {
		puntos = -min(int64(100), c.saldo)
		motivo = "Penalización por inasistencia"
	} else if c.saldo > math.MaxInt64-puntos {
		return c, MovimientoPuntos{}, ErrOperacionInvalida
	}
	m, err := NuevoMovimiento(DatosMovimiento{ID: id, CuentaID: c.id, Tipo: h.Tipo, OrigenID: h.OrigenID, Puntos: puntos, Secuencia: c.secuencia + 1, Motivo: motivo, Referencia: h.ReservaID, OcurridoEn: h.OcurridoEn, AplicadoEn: ahora})
	if err != nil {
		return c, MovimientoPuntos{}, err
	}
	c.saldo += puntos
	c.secuencia++
	return c, m, nil
}
