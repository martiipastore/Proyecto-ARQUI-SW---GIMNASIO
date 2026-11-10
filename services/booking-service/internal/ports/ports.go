package ports

import (
	"context"
	"errors"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/domain"
)

// ErrNoDisponible indica que una dependencia (base o members-service) no pudo
// confirmar la operación. Las validaciones de membresía fallan cerradas (ADR-005 v2).
var ErrNoDisponible = errors.New("dependencia no disponible")

// Operacion identifica una escritura idempotente (RN-15): misma clave y misma
// huella devuelven el resultado original; otra huella es un conflicto.
type Operacion struct {
	Actor  string
	Clave  string
	Tipo   string
	Huella string
}

// Reloj permite fijar el tiempo en los tests (RN-35).
type Reloj interface {
	Ahora() time.Time
}

// Members es el puerto hacia members-service (users.v1, APIs internas).
type Members interface {
	Vigencia(ctx context.Context, alumnoID string, fechas []string) (domain.Vigencia, error)
	AlumnoPorDNI(ctx context.Context, dni string) (domain.Persona, error)
	Alumnos(ctx context.Context, ids []string) (map[string]domain.Persona, error)
	Profesores(ctx context.Context, ids []string) (map[string]domain.Persona, error)
}

// Catalogo persiste actividades, horarios y clases (HU-06 a HU-08).
type Catalogo interface {
	Actividades(ctx context.Context) ([]domain.Actividad, error)
	Actividad(ctx context.Context, id string) (domain.Actividad, error)
	CrearActividad(ctx context.Context, actor string, a domain.Actividad) (domain.Actividad, error)
	// ModificarActividad bloquea la actividad y le pasa a cambiar si tiene clases
	// futuras con reservas confirmadas (CA-06.3).
	ModificarActividad(ctx context.Context, actor, id string, cambiar func(actual domain.Actividad, conReservasFuturas bool) (domain.Actividad, error)) (domain.Actividad, error)
	Horarios(ctx context.Context, actividadID string) ([]domain.Horario, error)
	// CrearHorario rechaza con domain.ErrSuperposicionProfesor si el profesor ya
	// tiene un horario que se pisa (RN-08, CA-07.4).
	CrearHorario(ctx context.Context, actor string, h domain.Horario) (domain.Horario, error)
	// GuardarClases inserta sin duplicar por actividad e inicio (CA-07.5) y
	// devuelve cuántas creó.
	GuardarClases(ctx context.Context, actor string, clases []domain.Clase) (int, error)
	BuscarClases(ctx context.Context, f domain.FiltroClases, alumnoID string) (domain.PaginaClases, error)
	Clase(ctx context.Context, id, alumnoID string) (domain.Clase, error)
	ClasesDeProfesor(ctx context.Context, profesorID string, desde time.Time) ([]domain.Clase, error)
	ReservasDeClase(ctx context.Context, claseID string) ([]domain.Reserva, error)
	// ModificarClase bloquea la clase, aplica validar y guarda la nueva capacidad o
	// la cancelación con sus reservas (CA-08.1 a CA-08.4, RN-32).
	CambiarCapacidad(ctx context.Context, actor, id string, capacidad int, validar func(domain.Clase, domain.Actividad) error) (domain.Clase, error)
	CancelarClase(ctx context.Context, actor, id, motivo string, validar func(domain.Clase) error) (domain.Clase, error)
}

// Reservas persiste reservas, ingresos y cierres con sus invariantes (RN-13,
// RN-14, RN-39, RN-40, RN-21), las claves de idempotencia y el outbox.
type Reservas interface {
	// Previa devuelve el resultado guardado de una operación ya hecha con la misma
	// clave; destino debe ser un puntero al tipo de resultado.
	Previa(ctx context.Context, op Operacion, destino any) (bool, error)
	// Reservar bloquea la clase, aplica validar sobre su estado actual y verifica
	// duplicado, turno diario de Musculación y superposición en la misma transacción.
	Reservar(ctx context.Context, op Operacion, claseID string, validar func(domain.Clase) error) (domain.Reserva, bool, error)
	// CancelarReserva bloquea la reserva y su clase y aplica validar, que devuelve
	// sinCambios cuando ya estaba cancelada.
	CancelarReserva(ctx context.Context, op Operacion, reservaID, motivo string, validar func(domain.Reserva) (bool, error)) (domain.Reserva, bool, error)
	MisReservas(ctx context.Context, alumnoID, estado string, pagina, tamano int) (domain.PaginaReservas, error)
	// RegistrarIngreso bloquea las reservas del alumno cercanas a now, aplica
	// elegir y, si no es reingreso, la pasa a ASISTIDA con su asistencia y el
	// evento asistencia.registrada en el outbox (RN-19 a RN-21).
	RegistrarIngreso(ctx context.Context, op Operacion, alumnoID string, now time.Time, completar func([]domain.Reserva) (domain.Ingreso, error)) (domain.Ingreso, bool, error)
	// CerrarClases finaliza las clases terminadas, pasa a AUSENTE sus reservas
	// confirmadas y escribe inasistencia.registrada en el outbox (RN-38, CA-22.8).
	CerrarClases(ctx context.Context, now time.Time) (clases int, ausentes int, err error)
}
