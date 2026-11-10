package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// TopeMusculacion y TopeGrupal son las capacidades máximas (RN-07, RN-08, CA-06.2).
	TopeMusculacion = 50
	TopeGrupal      = 30
	// PuntosAsistencia es fijo para toda actividad (RN-20, CA-06.4).
	PuntosAsistencia = 500
	// PenalizacionAusencia es el máximo que se descuenta por inasistencia (RN-38).
	PenalizacionAusencia = 100
	// DuracionMusculacion es la duración de cada turno de Musculación (RN-07).
	DuracionMusculacion = 120
)

// FranjasMusculacion son los inicios de los 7 turnos de Musculación, en minutos
// desde las 00:00 (08:00, 10:00, …, 20:00; RN-07, CA-07.1).
var FranjasMusculacion = []int{8 * 60, 10 * 60, 12 * 60, 14 * 60, 16 * 60, 18 * 60, 20 * 60}

// Zona es la zona horaria de las reglas de negocio (S-01, RN-35).
var Zona = cargarZona()

func cargarZona() *time.Location {
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		// Argentina no usa horario de verano: el desplazamiento fijo es equivalente.
		return time.FixedZone("ART", -3*60*60)
	}
	return loc
}

// Actividad es la oferta del gimnasio (HU-06).
type Actividad struct {
	ID              string
	Nombre          string
	CapacidadMaxima int
	Musculacion     bool
	Activa          bool
}

// Tope devuelve la capacidad máxima admitida para la actividad.
func (a Actividad) Tope() int {
	if a.Musculacion {
		return TopeMusculacion
	}
	return TopeGrupal
}

// NormalizarNombre limpia espacios y valida 1 a 60 caracteres.
func NormalizarNombre(nombre string) (string, error) {
	nombre = strings.Join(strings.Fields(nombre), " ")
	if n := utf8.RuneCountInString(nombre); n < 1 || n > 60 {
		return "", ErrValidacion
	}
	return nombre, nil
}

// Validar aplica RN-07, RN-08 y CA-06.2 a la configuración de la actividad.
func (a Actividad) Validar() error {
	if _, err := NormalizarNombre(a.Nombre); err != nil {
		return err
	}
	if a.CapacidadMaxima < 1 || a.CapacidadMaxima > a.Tope() {
		return ErrValidacion
	}
	return nil
}

// Horario es la regla recurrente a partir de la cual se generan clases (HU-07).
type Horario struct {
	ID              string
	ActividadID     string
	DiaSemana       int // 1 lunes … 7 domingo (ISO 8601)
	InicioMinutos   int // minutos desde las 00:00, hora de Argentina
	DuracionMinutos int
	ProfesorID      string
	ProfesorNombre  string
	Capacidad       int
}

// FinMinutos es el minuto del día en que termina el horario.
func (h Horario) FinMinutos() int { return h.InicioMinutos + h.DuracionMinutos }

// ValidarHorario aplica RN-07, RN-08 y CA-07.3 para la actividad dada. Que el
// responsable sea un PROFESOR activo lo confirma members-service (CA-07.6).
func ValidarHorario(a Actividad, h Horario) error {
	if !a.Activa || h.DiaSemana < 1 || h.DiaSemana > 7 || h.Capacidad < 1 || h.Capacidad > a.CapacidadMaxima || h.ProfesorID == "" {
		return ErrValidacion
	}
	if a.Musculacion {
		if h.DuracionMinutos != DuracionMusculacion || !esFranjaMusculacion(h.InicioMinutos) {
			return ErrValidacion
		}
		return nil
	}
	if h.DuracionMinutos < 15 || h.DuracionMinutos > 240 || h.InicioMinutos < 0 || h.FinMinutos() > 24*60 {
		return ErrValidacion
	}
	return nil
}

func esFranjaMusculacion(inicio int) bool {
	for _, f := range FranjasMusculacion {
		if f == inicio {
			return true
		}
	}
	return false
}

// SeSuperponeCon indica si dos horarios del mismo día se pisan (RN-08, CA-07.4).
// Un horario que termina a las 20:00 no se pisa con uno que empieza a las 20:00.
func (h Horario) SeSuperponeCon(o Horario) bool {
	return h.DiaSemana == o.DiaSemana && h.InicioMinutos < o.FinMinutos() && o.InicioMinutos < h.FinMinutos()
}

// ParseHora convierte "HH:MM" en minutos desde las 00:00.
func ParseHora(s string) (int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil || len(s) != 5 {
		return 0, ErrValidacion
	}
	return t.Hour()*60 + t.Minute(), nil
}

// FormatHora convierte minutos desde las 00:00 en "HH:MM".
func FormatHora(minutos int) string {
	return fmt.Sprintf("%02d:%02d", minutos/60, minutos%60)
}
