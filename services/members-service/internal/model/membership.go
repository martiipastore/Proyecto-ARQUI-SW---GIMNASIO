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

// HasDuration indica si el tipo define su período (meses o días).
func (t MembershipType) HasDuration() bool { return t.Months != nil || t.Days != nil }

// EndFor calcula el vencimiento inclusivo de un período que empieza en start
// (D-77, CA-03.1 y S-04): el día de vencimiento es vigente, por eso termina el
// día anterior al mismo día del período siguiente. Si ese mes no tiene el día
// de inicio, cubre hasta su último día (31/01 + 1 mes vence el 28/02 o 29/02).
// Devuelve false si el tipo no define duración.
func (t MembershipType) EndFor(start time.Time) (time.Time, bool) {
	switch {
	case t.Months != nil:
		months := *t.Months
		firstOfTarget := time.Date(start.Year(), start.Month()+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
		lastOfTarget := firstOfTarget.AddDate(0, 1, -1)
		if start.Day() > lastOfTarget.Day() {
			return lastOfTarget, true
		}
		return time.Date(firstOfTarget.Year(), firstOfTarget.Month(), start.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1), true
	case t.Days != nil:
		return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, *t.Days-1), true
	}
	return time.Time{}, false
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

// CoveringMembership devuelve la membresía ACTIVA que cubre la fecha (RN-01:
// inicio ≤ fecha ≤ vencimiento, día de vencimiento incluido, S-04), o nil. Las
// membresías activas de un alumno no se superponen (RN-02), así que hay a lo
// sumo una.
func CoveringMembership(memberships []Membership, date string) *Membership {
	for i := range memberships {
		m := memberships[i]
		if m.State == "ACTIVA" && m.Start <= date && date <= m.End {
			return &m
		}
	}
	return nil
}

// DateCoverage indica si una fecha está cubierta y por qué membresía.
type DateCoverage struct {
	Covered      bool    `json:"vigente"`
	MembershipID *string `json:"membresia_id"`
}

// Coverage es la respuesta de la consulta interna de vigencia que usa booking
// para reservar (ARCHITECTURE §4.2, RN-12): vigente hoy y en cada fecha pedida.
type Coverage struct {
	StudentActive bool                    `json:"alumno_activo"`
	CoveredToday  bool                    `json:"vigente_hoy"`
	CoveredOn     map[string]DateCoverage `json:"vigente_en"`
	EvaluatedAt   string                  `json:"evaluado_en"`
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
