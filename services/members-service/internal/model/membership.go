package model

import (
	"errors"
	"time"
	_ "time/tzdata"
)

var ErrMembershipNotFound = errors.New("membresía o usuario no encontrado")
var ErrMembershipOverlap = errors.New("membresías superpuestas")
var ErrMembershipTransition = errors.New("transición de membresía inválida")
var ErrMembershipInProgress = errors.New("operación de membresía en curso")
var ErrForbidden = errors.New("operación no autorizada")

type MembershipType struct {
	Code    string `json:"codigo"`
	Name    string `json:"nombre"`
	Months  *int   `json:"duracion_meses"`
	Days    *int   `json:"duracion_dias"`
	Classes *int   `json:"clases"`
	Enabled bool   `json:"habilitado"`
}

type Membership struct {
	ID         string         `json:"id"`
	UserID     string         `json:"usuario_id"`
	Type       MembershipType `json:"tipo"`
	Start      string         `json:"fecha_inicio"`
	End        string         `json:"fecha_vencimiento"`
	State      string         `json:"estado"`
	PreviousID *string        `json:"membresia_anterior_id"`
	Reason     string         `json:"motivo"`
	Active     bool           `json:"activa"`
	Condition  string         `json:"condicion"`
}

func (m Membership) At(today string) Membership {
	if m.State == "ACTIVA" && m.End < today {
		m.State = "VENCIDA"
	}
	m.Active = false
	switch {
	case m.State == "CANCELADA":
		m.Condition = "CANCELADA"
	case m.State == "VENCIDA":
		m.Condition = "VENCIDA"
	case m.Start > today:
		m.Condition = "PROGRAMADA"
	default:
		m.Active = true
		m.Condition = "VIGENTE"
	}
	return m
}

func ArgentinaDate(now time.Time) string {
	location, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		panic("zona horaria incorporada no disponible")
	}
	return now.In(location).Format("2006-01-02")
}

type MembershipPage struct {
	UserID        string       `json:"usuario_id"`
	ReferenceDate string       `json:"fecha_referencia"`
	Active        bool         `json:"activa"`
	Current       *Membership  `json:"membresia_vigente"`
	Items         []Membership `json:"items"`
	Total         int64        `json:"total"`
	Page          int          `json:"pagina"`
	Size          int          `json:"tamano"`
}

type MembershipInput struct {
	UserID   string `json:"usuario_id"`
	TypeCode string `json:"tipo_codigo"`
	Start    string `json:"fecha_inicio"`
	End      string `json:"fecha_vencimiento"`
	Reason   string `json:"motivo"`
}

type MembershipResult struct {
	Membership *Membership     `json:"membresia,omitempty"`
	Type       *MembershipType `json:"tipo,omitempty"`
}
