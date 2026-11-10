package domain

import (
	"sort"
	"time"
)

// Estados de una reserva (SPEC §7.3).
const (
	ReservaConfirmada = "CONFIRMADA"
	ReservaCancelada  = "CANCELADA"
	ReservaAsistida   = "ASISTIDA"
	ReservaAusente    = "AUSENTE"
)

// Motivos de cancelación registrados en la reserva.
const (
	MotivoCancelacionAlumno = "cancelada por el alumno"
	MotivoCancelacionAdmin  = "cancelada por el administrador"
	MotivoClaseCancelada    = "clase cancelada"
)

// AnticipacionCancelacion es el plazo mínimo para cancelar (RN-17, D-02).
const AnticipacionCancelacion = time.Hour

// Reserva de un alumno en una clase (HU-14 a HU-16).
type Reserva struct {
	ID                string
	ClaseID           string
	AlumnoID          string
	Estado            string
	CreadaEn          time.Time
	IngresoEn         *time.Time
	CanceladaEn       *time.Time
	MotivoCancelacion *string
	Clase             Clase
}

// ReservaConAlumno agrega el nombre que informa members-service (HU-21, D-22).
type ReservaConAlumno struct {
	Reserva
	AlumnoNombre *string
}

// PaginaReservas es un resultado paginado de reservas.
type PaginaReservas struct {
	Items  []Reserva
	Total  int
	Pagina int
	Tamano int
}

// ValidarCancelacion aplica RN-17 y CU-02. Devuelve sinCambios=true cuando la
// reserva ya estaba cancelada (CA-15.2): no es error y no libera cupo otra vez.
func (r Reserva) ValidarCancelacion(now time.Time) (sinCambios bool, err error) {
	switch r.Estado {
	case ReservaCancelada:
		return true, nil
	case ReservaConfirmada:
	default:
		return false, ErrTransicionInvalida
	}
	if !now.Before(r.Clase.Inicio.Add(-AnticipacionCancelacion)) {
		return false, ErrPlazoCancelacionVencido
	}
	return false, nil
}

// DiaMusculacion devuelve el día calendario que ocupa la reserva para RN-39, o
// cadena vacía si la clase no es de Musculación.
func DiaMusculacion(c Clase) string {
	if !c.Musculacion {
		return ""
	}
	return c.Fecha()
}

// Vigencia es lo que members-service confirma sobre el alumno (RN-01, RN-12).
type Vigencia struct {
	AlumnoActivo bool
	VigenteHoy   bool
	VenceHoy     *string
	VigenteEn    map[string]bool
}

// ValidarParaReservar aplica RN-12 (a) y (b) para la fecha de la clase.
func (v Vigencia) ValidarParaReservar(fecha string) error {
	if !v.AlumnoActivo {
		return ErrUsuarioInactivo
	}
	if !v.VigenteHoy || !v.VigenteEn[fecha] {
		return ErrMembresiaNoVigente
	}
	return nil
}

// Persona son los datos mínimos que members-service comparte (users.v1).
type Persona struct {
	ID       string
	Nombre   string
	Apellido *string
	Estado   string
}

// NombreCompleto une nombre y apellido.
func (p Persona) NombreCompleto() string {
	if p.Apellido == nil || *p.Apellido == "" {
		return p.Nombre
	}
	return p.Nombre + " " + *p.Apellido
}

// Ingreso es el resultado de un ingreso por DNI válido (HU-22, D-36 a D-38).
type Ingreso struct {
	Reserva   Reserva
	Reingreso bool
	Nombre    string
	Vence     *string
}

// ElegirReservaParaIngreso aplica D-37 a D-39 sobre las reservas CONFIRMADA y
// ASISTIDA del alumno. Prioriza la reserva confirmada con la ventana abierta que
// empieza antes; si no hay, una asistencia de una clase que todavía no terminó
// es un reingreso sin puntos (D-38). Si no, no puede pasar (D-39, CA-22.3/5).
func ElegirReservaParaIngreso(reservas []Reserva, now time.Time) (Reserva, bool, error) {
	ordenadas := append([]Reserva(nil), reservas...)
	sort.Slice(ordenadas, func(i, j int) bool { return ordenadas[i].Clase.Inicio.Before(ordenadas[j].Clase.Inicio) })
	for _, r := range ordenadas {
		if r.Estado != ReservaConfirmada || r.Clase.Estado == ClaseCancelada {
			continue
		}
		desde, hasta := r.Clase.VentanaIngreso()
		if !now.Before(desde) && !now.After(hasta) {
			return r, false, nil
		}
	}
	for _, r := range ordenadas {
		if r.Estado == ReservaAsistida && now.Before(r.Clase.Fin) {
			return r, true, nil
		}
	}
	return Reserva{}, false, ErrSinReservaParaHorario
}
