// Etapa 4 de la entrega final — casos de uso de booking con dobles en memoria:
// RN-12, RN-15, RN-17, CA-07.6, CA-14.2/3, CA-15.4, CA-22.3/4/7 y ADR-005 v2
// (falla cerrada). Las invariantes de concurrencia se prueban contra PostgreSQL
// en internal/adapters/postgres.
package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/ports"
)

const (
	admin    = "00000000-0000-4000-8000-000000000001"
	alumno   = "00000000-0000-4000-8000-000000000002"
	otro     = "00000000-0000-4000-8000-000000000003"
	profesor = "00000000-0000-4000-8000-000000000004"
	claseID  = "00000000-0000-4000-8000-000000000010"
	actID    = "00000000-0000-4000-8000-000000000020"
	clave    = "clave-de-prueba-1"
)

type reloj struct{ t time.Time }

func (r reloj) Ahora() time.Time { return r.t }

type members struct {
	vigencia   domain.Vigencia
	persona    domain.Persona
	profesores map[string]domain.Persona
	err        error
}

func (m members) Vigencia(context.Context, string, []string) (domain.Vigencia, error) {
	return m.vigencia, m.err
}
func (m members) AlumnoPorDNI(context.Context, string) (domain.Persona, error) {
	if m.err != nil {
		return domain.Persona{}, m.err
	}
	if m.persona.ID == "" {
		return domain.Persona{}, domain.ErrNoEncontrado
	}
	return m.persona, nil
}
func (m members) Alumnos(context.Context, []string) (map[string]domain.Persona, error) {
	return nil, m.err
}
func (m members) Profesores(context.Context, []string) (map[string]domain.Persona, error) {
	return m.profesores, m.err
}

type catalogo struct {
	ports.Catalogo
	clase     domain.Clase
	actividad domain.Actividad
	horario   *domain.Horario
}

func (c *catalogo) Clase(context.Context, string, string) (domain.Clase, error) { return c.clase, nil }
func (c *catalogo) Actividad(context.Context, string) (domain.Actividad, error) {
	return c.actividad, nil
}
func (c *catalogo) CrearHorario(_ context.Context, _ string, h domain.Horario) (domain.Horario, error) {
	c.horario = &h
	return h, nil
}

type reservas struct {
	ports.Reservas
	clase     domain.Clase
	reserva   domain.Reserva
	previa    bool
	creadas   int
	ingresos  []domain.Reserva
	asistidas int
}

func (r *reservas) Previa(_ context.Context, _ ports.Operacion, destino any) (bool, error) {
	if r.previa {
		*(destino.(*domain.Reserva)) = r.reserva
	}
	return r.previa, nil
}
func (r *reservas) Reservar(_ context.Context, op ports.Operacion, _ string, validar func(domain.Clase) error) (domain.Reserva, bool, error) {
	if err := validar(r.clase); err != nil {
		return domain.Reserva{}, false, err
	}
	r.creadas++
	return domain.Reserva{ID: "r", AlumnoID: op.Actor, Estado: domain.ReservaConfirmada, Clase: r.clase}, false, nil
}
func (r *reservas) CancelarReserva(_ context.Context, _ ports.Operacion, _, _ string, validar func(domain.Reserva) (bool, error)) (domain.Reserva, bool, error) {
	_, err := validar(r.reserva)
	return r.reserva, false, err
}
func (r *reservas) RegistrarIngreso(_ context.Context, _ ports.Operacion, _ string, _ time.Time, completar func([]domain.Reserva) (domain.Ingreso, error)) (domain.Ingreso, bool, error) {
	in, err := completar(r.ingresos)
	if err == nil && !in.Reingreso {
		r.asistidas++
	}
	return in, false, err
}

func escenario(t *testing.T) (*Servicio, *catalogo, *reservas, *members, time.Time) {
	t.Helper()
	ahora := time.Date(2026, 11, 10, 15, 0, 0, 0, domain.Zona)
	clase := domain.Clase{ID: claseID, Inicio: ahora.Add(4 * time.Hour), Fin: ahora.Add(5 * time.Hour), Capacidad: 30, Ocupacion: 10, Estado: domain.ClaseProgramada}
	cat := &catalogo{clase: clase, actividad: domain.Actividad{ID: actID, Nombre: "Zumba", CapacidadMaxima: 30, Activa: true}}
	res := &reservas{clase: clase}
	m := &members{vigencia: domain.Vigencia{AlumnoActivo: true, VigenteHoy: true, VigenteEn: map[string]bool{clase.Fecha(): true}}}
	return New(cat, res, m, reloj{ahora}), cat, res, m, ahora
}

func TestCU01Reservar(t *testing.T) {
	s, _, res, m, _ := escenario(t)
	yo := Actor{ID: alumno, Rol: RolAlumno}
	if _, _, err := s.Reservar(context.Background(), yo, clave, claseID); err != nil || res.creadas != 1 {
		t.Fatalf("CA-14.1: reserva válida: %v", err)
	}
	m.vigencia.VigenteEn = map[string]bool{}
	if _, _, err := s.Reservar(context.Background(), yo, clave, claseID); !errors.Is(err, domain.ErrMembresiaNoVigente) {
		t.Errorf("CA-14.3: la membresía no cubre la fecha: %v", err)
	}
	m.err = errors.New("members caído")
	if _, _, err := s.Reservar(context.Background(), yo, clave, claseID); !errors.Is(err, ports.ErrNoDisponible) || res.creadas != 1 {
		t.Errorf("ADR-005 v2: sin confirmación de members no se reserva: %v", err)
	}
	if _, _, err := s.Reservar(context.Background(), Actor{ID: admin, Rol: RolAdministrador}, clave, claseID); !errors.Is(err, domain.ErrSinPermiso) {
		t.Error("D-15: el administrador no reserva por un alumno")
	}
	if _, _, err := s.Reservar(context.Background(), yo, "corta", claseID); !errors.Is(err, domain.ErrValidacion) {
		t.Error("la clave de idempotencia tiene entre 8 y 128 caracteres")
	}
}

func TestRN15Replay(t *testing.T) {
	s, _, res, m, _ := escenario(t)
	res.previa = true
	res.reserva = domain.Reserva{ID: "original"}
	m.err = errors.New("members caído")
	r, replay, err := s.Reservar(context.Background(), Actor{ID: alumno, Rol: RolAlumno}, clave, claseID)
	if err != nil || !replay || r.ID != "original" || res.creadas != 0 {
		t.Errorf("CA-14.7: la misma clave devuelve la reserva original sin efectos: %v", err)
	}
}

func TestCA15_4CancelarAjena(t *testing.T) {
	s, _, res, _, ahora := escenario(t)
	res.reserva = domain.Reserva{ID: "r", AlumnoID: otro, Estado: domain.ReservaConfirmada, Clase: domain.Clase{Inicio: ahora.Add(4 * time.Hour)}}
	if _, _, err := s.CancelarReserva(context.Background(), Actor{ID: alumno, Rol: RolAlumno}, clave, claseID); !errors.Is(err, domain.ErrSinPermiso) {
		t.Errorf("CA-15.4: no se cancela la reserva de otro alumno: %v", err)
	}
	if _, _, err := s.CancelarReserva(context.Background(), Actor{ID: admin, Rol: RolAdministrador}, clave, claseID); err != nil {
		t.Errorf("D-15: el administrador cancela en nombre del alumno: %v", err)
	}
}

func TestCA07_6ResponsableProfesor(t *testing.T) {
	s, cat, _, m, _ := escenario(t)
	in := NuevoHorario{DiaSemana: 2, Inicio: "19:00", DuracionMinutos: 60, ProfesorID: profesor, Capacidad: 30}
	a := Actor{ID: admin, Rol: RolAdministrador}
	m.profesores = map[string]domain.Persona{}
	if _, err := s.CrearHorario(context.Background(), a, actID, in); !errors.Is(err, domain.ErrValidacion) || cat.horario != nil {
		t.Error("CA-07.6: un nutricionista (u otra persona que no es profesor) no es responsable")
	}
	m.profesores = map[string]domain.Persona{profesor: {ID: profesor, Nombre: "Lucía", Estado: "INACTIVO"}}
	if _, err := s.CrearHorario(context.Background(), a, actID, in); !errors.Is(err, domain.ErrValidacion) {
		t.Error("RN-08: el profesor debe estar activo")
	}
	apellido := "Gómez"
	m.profesores = map[string]domain.Persona{profesor: {ID: profesor, Nombre: "Lucía", Apellido: &apellido, Estado: "ACTIVO"}}
	h, err := s.CrearHorario(context.Background(), a, actID, in)
	if err != nil || h.ProfesorNombre != "Lucía Gómez" || h.InicioMinutos != 19*60 {
		t.Errorf("horario válido: %+v %v", h, err)
	}
}

func TestHU22Ingreso(t *testing.T) {
	s, _, res, m, ahora := escenario(t)
	a := Actor{ID: admin, Rol: RolAdministrador}
	if _, _, err := s.RegistrarIngreso(context.Background(), a, clave, "40123456"); !errors.Is(err, domain.ErrNoEncontrado) {
		t.Errorf("CA-22.7: DNI inexistente: %v", err)
	}
	m.persona = domain.Persona{ID: alumno, Nombre: "Martina", Estado: "ACTIVO"}
	m.vigencia.VigenteHoy = false
	var rechazo RechazoIngreso
	if _, _, err := s.RegistrarIngreso(context.Background(), a, clave, "40123456"); !errors.Is(err, domain.ErrMembresiaNoVigente) || !errors.As(err, &rechazo) || rechazo.Nombre != "Martina" {
		t.Errorf("CA-22.4: membresía vencida, nombre en rojo: %v", err)
	}
	m.vigencia.VigenteHoy = true
	if _, _, err := s.RegistrarIngreso(context.Background(), a, clave, "40123456"); !errors.Is(err, domain.ErrSinReservaParaHorario) || res.asistidas != 0 {
		t.Errorf("CA-22.3/D-39: sin reserva no pasa: %v", err)
	}
	res.ingresos = []domain.Reserva{{ID: "r", Estado: domain.ReservaConfirmada, Clase: domain.Clase{Inicio: ahora.Add(10 * time.Minute), Fin: ahora.Add(70 * time.Minute), Estado: domain.ClaseProgramada}}}
	in, _, err := s.RegistrarIngreso(context.Background(), a, clave, "40123456")
	if err != nil || in.Reingreso || in.Nombre != "Martina" || res.asistidas != 1 {
		t.Errorf("CA-22.1: ingreso válido registra la asistencia: %+v %v", in, err)
	}
	if _, _, err := s.RegistrarIngreso(context.Background(), Actor{ID: alumno, Rol: RolAlumno}, clave, "40123456"); !errors.Is(err, domain.ErrSinPermiso) {
		t.Error("D-82: solo el administrador registra ingresos")
	}
	m.persona.Estado = "INACTIVO"
	if _, _, err := s.RegistrarIngreso(context.Background(), a, clave, "40123456"); !errors.Is(err, domain.ErrUsuarioInactivo) {
		t.Errorf("CA-22.4: usuario inactivo: %v", err)
	}
}
