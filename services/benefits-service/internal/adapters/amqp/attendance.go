// Package amqp traduce envelopes de Booking al puerto de entrada de Benefits.
package amqp

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"time"
)

type Handler interface {
	Aplicar(context.Context, domain.HechoBooking) error
}
type Adapter struct {
	App       Handler
	EventType string
}

func (a Adapter) Handle(ctx context.Context, body []byte) error {
	var e struct {
		ID       string    `json:"event_id"`
		Type     string    `json:"event_type"`
		Version  int       `json:"event_version"`
		Producer string    `json:"producer"`
		Occurred time.Time `json:"occurred_at"`
		Data     struct {
			Asistencia string `json:"asistencia_id"`
			Reserva    string `json:"reserva_id"`
			Alumno     string `json:"alumno_id"`
			Clase      string `json:"clase_id"`
			Puntos     int64  `json:"puntos"`
			Penalty    int64  `json:"puntos_penalizacion"`
			Resultado  string `json:"resultado"`
			Origen     string `json:"origen"`
		} `json:"data"`
	}
	if len(body) > 64<<10 || json.Unmarshal(body, &e) != nil || e.Version != 1 || e.Producer != "booking-service" || e.Type != a.EventType {
		return domain.ErrOperacionInvalida
	}
	h := domain.HechoBooking{EventoID: e.ID, AlumnoID: e.Data.Alumno, ReservaID: e.Data.Reserva, ClaseID: e.Data.Clase, OcurridoEn: e.Occurred}
	switch e.Type {
	case "asistencia.registrada":
		if e.Data.Puntos != 500 || e.Data.Resultado != "ASISTIO" || e.Data.Origen != "INGRESO_DNI" {
			return domain.ErrOperacionInvalida
		}
		h.Tipo = domain.Asistencia
		h.OrigenID = e.Data.Asistencia
	case "inasistencia.registrada":
		if e.Data.Penalty != 100 {
			return domain.ErrOperacionInvalida
		}
		h.Tipo = domain.PenalizacionInasistencia
		h.OrigenID = e.Data.Reserva
	default:
		return domain.ErrOperacionInvalida
	}
	return a.App.Aplicar(ctx, h)
}

func Permanent(err error) bool {
	return errors.Is(err, domain.ErrOperacionInvalida) || errors.Is(err, domain.ErrConflictoIdempotencia)
}
