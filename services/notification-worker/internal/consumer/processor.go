package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/notification-worker/internal/mailer"
)

var ErrInvalid = errors.New("evento inválido")

type Event struct {
	ID       string    `json:"event_id"`
	Type     string    `json:"event_type"`
	Version  int       `json:"event_version"`
	Occurred time.Time `json:"occurred_at"`
	Data     struct {
		Membership     string `json:"membresia_id"`
		Student        string `json:"alumno_id"`
		Name           string `json:"alumno_nombre"`
		Email          string `json:"alumno_email"`
		MembershipType string `json:"tipo_membresia"`
		Start          string `json:"fecha_inicio"`
		End            string `json:"fecha_vencimiento"`
		Operation      string `json:"operacion"`
	} `json:"data"`
}

type Store interface {
	Claim(context.Context, string, []byte) (bool, error)
	Finish(context.Context, string, bool) error
}
type Mailer interface {
	Send(context.Context, mailer.Email) error
}

type Processor struct {
	Store  Store
	Mailer Mailer
}

// Process implementa RN-05: un evento crea una notificación lógica durable.
// SMTP no ofrece transacción distribuida: una caída tras aceptar DATA puede
// repetir el envío físico; Message-ID permanece estable y se documenta el límite.
func (p Processor) Process(ctx context.Context, body []byte) error {
	var e Event
	if json.Unmarshal(body, &e) != nil || e.Type != "membresia.activada" || e.Version != 1 || e.Occurred.IsZero() || !validID(e.ID) || !validID(e.Data.Membership) || !validID(e.Data.Student) || strings.ContainsAny(e.Data.Email, "\r\n") {
		return ErrInvalid
	}
	if _, err := mail.ParseAddress(e.Data.Email); err != nil {
		return ErrInvalid
	}
	start, err := time.Parse(time.DateOnly, e.Data.Start)
	if err != nil {
		return ErrInvalid
	}
	end, err := time.Parse(time.DateOnly, e.Data.End)
	if err != nil || end.Before(start) || (e.Data.Operation != "ASIGNACION" && e.Data.Operation != "RENOVACION") || e.Data.Name == "" || e.Data.MembershipType == "" {
		return ErrInvalid
	}
	// RFC3339 permite distintos offsets para el mismo instante. La huella
	// debe deduplicar el hecho aunque un replay lo vuelva a serializar en UTC.
	e.Occurred = e.Occurred.UTC()
	canonical, err := json.Marshal(e)
	if err != nil {
		return ErrInvalid
	}
	claimed, err := p.Store.Claim(ctx, e.ID, canonical)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	err = p.Mailer.Send(ctx, mailer.Email{To: e.Data.Email, Subject: "Confirmación de membresía", MessageID: e.ID, Body: fmt.Sprintf("Hola %s,\nTu membresía %s fue registrada (%s).\nVigencia: %s a %s.\n", e.Data.Name, e.Data.MembershipType, e.Data.Operation, e.Data.Start, e.Data.End)})
	if err != nil {
		if saved := p.Store.Finish(ctx, e.ID, false); saved != nil {
			return saved
		}
		return errors.New("envío SMTP pendiente")
	}
	return p.Store.Finish(ctx, e.ID, true)
}

func validID(s string) bool {
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
