package domain

import (
	"sort"
	"time"
)

// Estados de una clase (SPEC §7.2). Se persisten PROGRAMADA, CANCELADA y
// FINALIZADA (cierre automático); EN_CURSO se deriva del reloj (RN-35).
const (
	ClaseProgramada = "PROGRAMADA"
	ClaseEnCurso    = "EN_CURSO"
	ClaseFinalizada = "FINALIZADA"
	ClaseCancelada  = "CANCELADA"
)

// MaxDiasGeneracion limita el período que se genera en una sola operación.
const MaxDiasGeneracion = 62

// Clase es una ocurrencia concreta de un horario (HU-07, HU-13).
type Clase struct {
	ID              string
	ActividadID     string
	ActividadNombre string
	Musculacion     bool
	HorarioID       string
	Inicio          time.Time
	Fin             time.Time
	ProfesorID      string
	ProfesorNombre  string
	Capacidad       int
	Ocupacion       int
	Estado          string
	ReservadaPorMi  bool
}

// EstadoEn devuelve el estado efectivo en el instante now: una clase con inicio
// ≤ now se trata como iniciada aunque el cierre automático se retrase (SPEC §7.2).
func (c Clase) EstadoEn(now time.Time) string {
	switch {
	case c.Estado == ClaseCancelada:
		return ClaseCancelada
	case c.Estado == ClaseFinalizada || !now.Before(c.Fin):
		return ClaseFinalizada
	case !now.Before(c.Inicio):
		return ClaseEnCurso
	default:
		return ClaseProgramada
	}
}

// Disponibles es capacidad − ocupación (RN-11).
func (c Clase) Disponibles() int {
	if d := c.Capacidad - c.Ocupacion; d > 0 {
		return d
	}
	return 0
}

// Reservable indica si se ofrece reservar (CA-13.2, CA-13.3).
func (c Clase) Reservable(now time.Time) bool {
	return c.EstadoEn(now) == ClaseProgramada && c.Disponibles() > 0
}

// ValidarReserva aplica RN-12 (c) y (d) sobre la clase bloqueada en la base.
func (c Clase) ValidarReserva(now time.Time) error {
	switch c.EstadoEn(now) {
	case ClaseCancelada:
		return ErrClaseCancelada
	case ClaseProgramada:
	default:
		return ErrClaseNoFutura
	}
	if c.Disponibles() == 0 {
		return ErrSinCupo
	}
	return nil
}

// ValidarCapacidad aplica RN-10 y CA-08.1/2/4.
func (c Clase) ValidarCapacidad(nueva, tope int, now time.Time) error {
	if c.EstadoEn(now) != ClaseProgramada {
		return ErrTransicionInvalida
	}
	if nueva < 1 || nueva > tope || nueva < c.Ocupacion {
		return ErrValidacion
	}
	return nil
}

// ValidarCancelacion aplica RN-32 y CA-08.4.
func (c Clase) ValidarCancelacion(now time.Time) error {
	if c.EstadoEn(now) != ClaseProgramada {
		return ErrTransicionInvalida
	}
	return nil
}

// Franja devuelve el turno de Musculación ("08:00-10:00"); nil en otras actividades.
func (c Clase) Franja() *string {
	if !c.Musculacion {
		return nil
	}
	f := c.Inicio.In(Zona).Format("15:04") + "-" + c.Fin.In(Zona).Format("15:04")
	return &f
}

// Fecha es el día calendario de la clase en la zona del gimnasio.
func (c Clase) Fecha() string { return c.Inicio.In(Zona).Format(time.DateOnly) }

// VentanaIngreso es el intervalo en que el ingreso por DNI cuenta como asistencia:
// desde 15 minutos antes del inicio hasta 30 minutos antes del fin (D-37).
func (c Clase) VentanaIngreso() (desde, hasta time.Time) {
	return c.Inicio.Add(-15 * time.Minute), c.Fin.Add(-30 * time.Minute)
}

// Superponen indica si dos intervalos se pisan; los consecutivos no (RN-40, D-18).
func Superponen(inicioA, finA, inicioB, finB time.Time) bool {
	return inicioA.Before(finB) && inicioB.Before(finA)
}

// FiltroClases son los criterios de búsqueda (HU-13).
type FiltroClases struct {
	Desde, Hasta    time.Time
	ActividadID     string
	Franja          string // "HH:MM" de inicio, hora de Argentina
	SoloDisponibles bool
	Pagina, Tamano  int
}

// PaginaClases es un resultado paginado de clases.
type PaginaClases struct {
	Items  []Clase
	Total  int
	Pagina int
	Tamano int
}

// ParseFecha valida una fecha calendario AAAA-MM-DD en la zona del gimnasio.
func ParseFecha(s string) (time.Time, error) {
	t, err := time.ParseInLocation(time.DateOnly, s, Zona)
	if err != nil {
		return time.Time{}, ErrValidacion
	}
	return t, nil
}

// GenerarClases crea las clases de los horarios entre desde y hasta inclusive
// (RN-09, CA-07.1/2). Solo genera clases futuras; la base descarta las que ya
// existen para la misma actividad y fecha-hora de inicio (CA-07.5).
func GenerarClases(a Actividad, horarios []Horario, desde, hasta string, now time.Time) ([]Clase, error) {
	inicio, err := ParseFecha(desde)
	if err != nil {
		return nil, err
	}
	fin, err := ParseFecha(hasta)
	if err != nil {
		return nil, err
	}
	if fin.Before(inicio) || fin.Sub(inicio) > (MaxDiasGeneracion-1)*24*time.Hour {
		return nil, ErrValidacion
	}
	if !a.Activa {
		return nil, ErrValidacion
	}
	var clases []Clase
	for dia := inicio; !dia.After(fin); dia = dia.AddDate(0, 0, 1) {
		semana := int(dia.Weekday())
		if semana == 0 {
			semana = 7
		}
		for _, h := range horarios {
			if h.DiaSemana != semana || h.ActividadID != a.ID {
				continue
			}
			comienzo := time.Date(dia.Year(), dia.Month(), dia.Day(), h.InicioMinutos/60, h.InicioMinutos%60, 0, 0, Zona)
			if !comienzo.After(now) {
				continue
			}
			clases = append(clases, Clase{
				ActividadID:     a.ID,
				ActividadNombre: a.Nombre,
				Musculacion:     a.Musculacion,
				HorarioID:       h.ID,
				Inicio:          comienzo.UTC(),
				Fin:             comienzo.Add(time.Duration(h.DuracionMinutos) * time.Minute).UTC(),
				ProfesorID:      h.ProfesorID,
				ProfesorNombre:  h.ProfesorNombre,
				Capacidad:       h.Capacidad,
				Estado:          ClaseProgramada,
			})
		}
	}
	sort.Slice(clases, func(i, j int) bool { return clases[i].Inicio.Before(clases[j].Inicio) })
	return clases, nil
}
