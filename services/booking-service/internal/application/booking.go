package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/ports"
)

// Roles que inyecta el gateway (ADR-006).
const (
	RolAdministrador = "ADMINISTRADOR"
	RolAlumno        = "ALUMNO"
	RolProfesor      = "PROFESOR"
)

var (
	patronUUID  = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	patronClave = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)
	patronDNI   = regexp.MustCompile(`^[0-9]{7,8}$`)
)

// Actor es la identidad verificada por el gateway.
type Actor struct {
	ID  string
	Rol string
}

// RechazoIngreso es un ingreso rechazado que igual muestra el nombre en rojo (D-36).
type RechazoIngreso struct {
	Motivo error
	Nombre string
}

func (r RechazoIngreso) Error() string { return r.Motivo.Error() }
func (r RechazoIngreso) Unwrap() error { return r.Motivo }

// DetalleClase es la clase con sus reservas cuando el actor puede verlas (CA-21.1, CA-35.1/2).
type DetalleClase struct {
	Clase    domain.Clase
	Reservas []domain.ReservaConAlumno
	Completo bool
}

// Servicio implementa los casos de uso de booking (puerto de entrada).
type Servicio struct {
	catalogo ports.Catalogo
	reservas ports.Reservas
	members  ports.Members
	reloj    ports.Reloj
}

// New cablea los casos de uso con sus puertos de salida.
func New(catalogo ports.Catalogo, reservas ports.Reservas, members ports.Members, reloj ports.Reloj) *Servicio {
	return &Servicio{catalogo: catalogo, reservas: reservas, members: members, reloj: reloj}
}

func huella(partes ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(partes, "\x1f")))
	return hex.EncodeToString(sum[:])
}

func (s *Servicio) admin(a Actor) error {
	if a.Rol != RolAdministrador || !patronUUID.MatchString(a.ID) {
		return domain.ErrSinPermiso
	}
	return nil
}

// dependencia convierte fallas técnicas en indisponibilidad sin ocultar errores de negocio.
func dependencia(err error) error {
	if err == nil || errors.Is(err, domain.ErrNoEncontrado) || errors.Is(err, domain.ErrValidacion) {
		return err
	}
	return errors.Join(ports.ErrNoDisponible, err)
}

// ---- Actividades y horarios (HU-06, HU-07) ----

// Actividades lista el catálogo (CA-06.1).
func (s *Servicio) Actividades(ctx context.Context) ([]domain.Actividad, error) {
	return s.catalogo.Actividades(ctx)
}

// CrearActividad da de alta una actividad grupal (RN-08); Musculación ya existe.
func (s *Servicio) CrearActividad(ctx context.Context, a Actor, nombre string, capacidad int) (domain.Actividad, error) {
	if err := s.admin(a); err != nil {
		return domain.Actividad{}, err
	}
	nombre, err := domain.NormalizarNombre(nombre)
	if err != nil {
		return domain.Actividad{}, err
	}
	nueva := domain.Actividad{Nombre: nombre, CapacidadMaxima: capacidad, Activa: true}
	if err = nueva.Validar(); err != nil {
		return domain.Actividad{}, err
	}
	return s.catalogo.CrearActividad(ctx, a.ID, nueva)
}

// ModificarActividad cambia nombre, capacidad máxima o estado (CA-06.2, CA-06.3, RN-34).
func (s *Servicio) ModificarActividad(ctx context.Context, a Actor, id string, nombre *string, capacidad *int, activa *bool) (domain.Actividad, error) {
	if err := s.admin(a); err != nil {
		return domain.Actividad{}, err
	}
	if !patronUUID.MatchString(id) || (nombre == nil && capacidad == nil && activa == nil) {
		return domain.Actividad{}, domain.ErrValidacion
	}
	return s.catalogo.ModificarActividad(ctx, a.ID, id, func(actual domain.Actividad, conReservas bool) (domain.Actividad, error) {
		if nombre != nil {
			n, err := domain.NormalizarNombre(*nombre)
			if err != nil {
				return actual, err
			}
			actual.Nombre = n
		}
		if capacidad != nil {
			actual.CapacidadMaxima = *capacidad
		}
		if activa != nil {
			if actual.Activa && !*activa && conReservas {
				return actual, domain.ErrTransicionInvalida
			}
			actual.Activa = *activa
		}
		return actual, actual.Validar()
	})
}

// Horarios lista los horarios de una actividad.
func (s *Servicio) Horarios(ctx context.Context, a Actor, actividadID string) ([]domain.Horario, error) {
	if err := s.admin(a); err != nil {
		return nil, err
	}
	if !patronUUID.MatchString(actividadID) {
		return nil, domain.ErrValidacion
	}
	if _, err := s.catalogo.Actividad(ctx, actividadID); err != nil {
		return nil, err
	}
	return s.catalogo.Horarios(ctx, actividadID)
}

// NuevoHorario son los datos de alta de un horario.
type NuevoHorario struct {
	DiaSemana       int
	Inicio          string
	DuracionMinutos int
	ProfesorID      string
	Capacidad       int
}

// CrearHorario aplica RN-07, RN-08, CA-07.3, CA-07.4 y CA-07.6.
func (s *Servicio) CrearHorario(ctx context.Context, a Actor, actividadID string, in NuevoHorario) (domain.Horario, error) {
	if err := s.admin(a); err != nil {
		return domain.Horario{}, err
	}
	if !patronUUID.MatchString(actividadID) || !patronUUID.MatchString(in.ProfesorID) {
		return domain.Horario{}, domain.ErrValidacion
	}
	actividad, err := s.catalogo.Actividad(ctx, actividadID)
	if err != nil {
		return domain.Horario{}, err
	}
	inicio, err := domain.ParseHora(in.Inicio)
	if err != nil {
		return domain.Horario{}, err
	}
	if actividad.Musculacion && in.DuracionMinutos == 0 {
		in.DuracionMinutos = domain.DuracionMusculacion
	}
	h := domain.Horario{ActividadID: actividadID, DiaSemana: in.DiaSemana, InicioMinutos: inicio, DuracionMinutos: in.DuracionMinutos, ProfesorID: in.ProfesorID, Capacidad: in.Capacidad}
	if err = domain.ValidarHorario(actividad, h); err != nil {
		return domain.Horario{}, err
	}
	// Solo un PROFESOR activo puede ser responsable; un nutricionista no figura (CA-07.6).
	profesores, err := s.members.Profesores(ctx, []string{in.ProfesorID})
	if err != nil {
		return domain.Horario{}, dependencia(err)
	}
	profesor, ok := profesores[in.ProfesorID]
	if !ok || profesor.Estado != "ACTIVO" {
		return domain.Horario{}, domain.ErrValidacion
	}
	h.ProfesorNombre = profesor.NombreCompleto()
	return s.catalogo.CrearHorario(ctx, a.ID, h)
}

// GenerarClases crea las clases de todas las actividades activas en el período
// (RN-09, CA-07.5). Devuelve cuántas clases nuevas se crearon.
func (s *Servicio) GenerarClases(ctx context.Context, a Actor, desde, hasta string) (int, error) {
	if err := s.admin(a); err != nil {
		return 0, err
	}
	actividades, err := s.catalogo.Actividades(ctx)
	if err != nil {
		return 0, err
	}
	now := s.reloj.Ahora()
	var clases []domain.Clase
	for _, actividad := range actividades {
		if !actividad.Activa {
			continue
		}
		horarios, err := s.catalogo.Horarios(ctx, actividad.ID)
		if err != nil {
			return 0, err
		}
		generadas, err := domain.GenerarClases(actividad, horarios, desde, hasta, now)
		if err != nil {
			return 0, err
		}
		clases = append(clases, generadas...)
	}
	if len(clases) == 0 {
		// Valida igual el período aunque no haya horarios.
		if _, err = domain.GenerarClases(domain.Actividad{Activa: true}, nil, desde, hasta, now); err != nil {
			return 0, err
		}
		return 0, nil
	}
	return s.catalogo.GuardarClases(ctx, a.ID, clases)
}

// ---- Clases (HU-08, HU-13, HU-21) ----

// BuscarClases lista clases con su ocupación (HU-13). Para un alumno marca las
// que ya reservó (CA-13.4).
func (s *Servicio) BuscarClases(ctx context.Context, a Actor, f domain.FiltroClases) (domain.PaginaClases, error) {
	if f.ActividadID != "" && !patronUUID.MatchString(f.ActividadID) {
		return domain.PaginaClases{}, domain.ErrValidacion
	}
	if f.Franja != "" {
		if _, err := domain.ParseHora(f.Franja); err != nil {
			return domain.PaginaClases{}, err
		}
	}
	// Por defecto: desde hoy y durante 15 días (hasta es exclusivo).
	if f.Desde.IsZero() {
		f.Desde, _ = domain.ParseFecha(s.reloj.Ahora().In(domain.Zona).Format(time.DateOnly))
	}
	if f.Hasta.IsZero() {
		f.Hasta = f.Desde.AddDate(0, 0, 15)
	}
	if f.Hasta.Before(f.Desde) || f.Hasta.Sub(f.Desde) > domain.MaxDiasGeneracion*24*time.Hour || f.Pagina < 1 || f.Tamano < 1 || f.Tamano > 100 {
		return domain.PaginaClases{}, domain.ErrValidacion
	}
	alumno := ""
	if a.Rol == RolAlumno {
		alumno = a.ID
	}
	return s.catalogo.BuscarClases(ctx, f, alumno)
}

// DetalleClase devuelve la clase y, para el administrador o el profesor
// responsable, sus reservas con el nombre del alumno (CA-21.1, CA-35.1/2). Si
// members-service no responde, el listado sale sin nombres (D-22).
func (s *Servicio) DetalleClase(ctx context.Context, a Actor, id string) (DetalleClase, error) {
	if !patronUUID.MatchString(id) {
		return DetalleClase{}, domain.ErrValidacion
	}
	alumno := ""
	if a.Rol == RolAlumno {
		alumno = a.ID
	}
	clase, err := s.catalogo.Clase(ctx, id, alumno)
	if err != nil {
		return DetalleClase{}, err
	}
	out := DetalleClase{Clase: clase}
	if a.Rol != RolAdministrador && !(a.Rol == RolProfesor && clase.ProfesorID == a.ID) {
		return out, nil
	}
	reservas, err := s.catalogo.ReservasDeClase(ctx, id)
	if err != nil {
		return DetalleClase{}, err
	}
	ids := make([]string, 0, len(reservas))
	for _, r := range reservas {
		ids = append(ids, r.AlumnoID)
	}
	var nombres map[string]domain.Persona
	if len(ids) > 0 {
		nombres, _ = s.members.Alumnos(ctx, ids)
	}
	out.Completo = true
	out.Reservas = make([]domain.ReservaConAlumno, 0, len(reservas))
	for _, r := range reservas {
		item := domain.ReservaConAlumno{Reserva: r}
		if p, ok := nombres[r.AlumnoID]; ok {
			nombre := p.NombreCompleto()
			item.AlumnoNombre = &nombre
		}
		out.Reservas = append(out.Reservas, item)
	}
	return out, nil
}

// MisClases lista las clases del profesor desde hoy (HU-21, CA-21.1).
func (s *Servicio) MisClases(ctx context.Context, a Actor) ([]domain.Clase, error) {
	if a.Rol != RolProfesor || !patronUUID.MatchString(a.ID) {
		return nil, domain.ErrSinPermiso
	}
	hoy, _ := domain.ParseFecha(s.reloj.Ahora().In(domain.Zona).Format(time.DateOnly))
	return s.catalogo.ClasesDeProfesor(ctx, a.ID, hoy)
}

// CambiarCapacidad aplica RN-10 y CA-08.1/2/4.
func (s *Servicio) CambiarCapacidad(ctx context.Context, a Actor, id string, capacidad int) (domain.Clase, error) {
	if err := s.admin(a); err != nil {
		return domain.Clase{}, err
	}
	if !patronUUID.MatchString(id) {
		return domain.Clase{}, domain.ErrValidacion
	}
	return s.catalogo.CambiarCapacidad(ctx, a.ID, id, capacidad, func(c domain.Clase, act domain.Actividad) error {
		return c.ValidarCapacidad(capacidad, act.CapacidadMaxima, s.reloj.Ahora())
	})
}

// CancelarClase aplica RN-32 y CA-08.3/4.
func (s *Servicio) CancelarClase(ctx context.Context, a Actor, id, motivo string) (domain.Clase, error) {
	if err := s.admin(a); err != nil {
		return domain.Clase{}, err
	}
	motivo = strings.TrimSpace(motivo)
	if !patronUUID.MatchString(id) || utf8.RuneCountInString(motivo) < 3 || utf8.RuneCountInString(motivo) > 200 {
		return domain.Clase{}, domain.ErrValidacion
	}
	return s.catalogo.CancelarClase(ctx, a.ID, id, motivo, func(c domain.Clase) error {
		return c.ValidarCancelacion(s.reloj.Ahora())
	})
}

// ---- Reservas (HU-14 a HU-16) ----

// Reservar aplica CU-01: idempotencia, membresía vigente hoy y en la fecha de la
// clase (falla cerrada si members no responde) y, en una sola transacción, estado,
// cupo, duplicado, turno diario de Musculación y superposición.
func (s *Servicio) Reservar(ctx context.Context, a Actor, clave, claseID string) (domain.Reserva, bool, error) {
	if a.Rol != RolAlumno || !patronUUID.MatchString(a.ID) {
		return domain.Reserva{}, false, domain.ErrSinPermiso
	}
	if !patronClave.MatchString(clave) || !patronUUID.MatchString(claseID) {
		return domain.Reserva{}, false, domain.ErrValidacion
	}
	op := ports.Operacion{Actor: a.ID, Clave: clave, Tipo: "RESERVAR", Huella: huella("RESERVAR", claseID)}
	var previa domain.Reserva
	if ok, err := s.reservas.Previa(ctx, op, &previa); err != nil || ok {
		return previa, ok, err
	}
	clase, err := s.catalogo.Clase(ctx, claseID, "")
	if err != nil {
		return domain.Reserva{}, false, err
	}
	if err = clase.ValidarReserva(s.reloj.Ahora()); err != nil {
		return domain.Reserva{}, false, err
	}
	vigencia, err := s.members.Vigencia(ctx, a.ID, []string{clase.Fecha()})
	if err != nil {
		return domain.Reserva{}, false, dependencia(err)
	}
	if err = vigencia.ValidarParaReservar(clase.Fecha()); err != nil {
		return domain.Reserva{}, false, err
	}
	return s.reservas.Reservar(ctx, op, claseID, func(c domain.Clase) error {
		return c.ValidarReserva(s.reloj.Ahora())
	})
}

// MisReservas lista las reservas del alumno (HU-16, CA-16.1).
func (s *Servicio) MisReservas(ctx context.Context, a Actor, estado string, pagina, tamano int) (domain.PaginaReservas, error) {
	if a.Rol != RolAlumno || !patronUUID.MatchString(a.ID) {
		return domain.PaginaReservas{}, domain.ErrSinPermiso
	}
	switch estado {
	case "", domain.ReservaConfirmada, domain.ReservaCancelada, domain.ReservaAsistida, domain.ReservaAusente:
	default:
		return domain.PaginaReservas{}, domain.ErrValidacion
	}
	if pagina < 1 || tamano < 1 || tamano > 100 {
		return domain.PaginaReservas{}, domain.ErrValidacion
	}
	return s.reservas.MisReservas(ctx, a.ID, estado, pagina, tamano)
}

// CancelarReserva aplica CU-02: el titular o el administrador, hasta 1 hora antes
// (RN-17, D-15); cancelar una cancelada informa su estado sin efectos (CA-15.2).
func (s *Servicio) CancelarReserva(ctx context.Context, a Actor, clave, id string) (domain.Reserva, bool, error) {
	if (a.Rol != RolAlumno && a.Rol != RolAdministrador) || !patronUUID.MatchString(a.ID) {
		return domain.Reserva{}, false, domain.ErrSinPermiso
	}
	if !patronClave.MatchString(clave) || !patronUUID.MatchString(id) {
		return domain.Reserva{}, false, domain.ErrValidacion
	}
	op := ports.Operacion{Actor: a.ID, Clave: clave, Tipo: "CANCELAR_RESERVA", Huella: huella("CANCELAR_RESERVA", id)}
	var previa domain.Reserva
	if ok, err := s.reservas.Previa(ctx, op, &previa); err != nil || ok {
		return previa, ok, err
	}
	motivo := domain.MotivoCancelacionAlumno
	if a.Rol == RolAdministrador {
		motivo = domain.MotivoCancelacionAdmin
	}
	return s.reservas.CancelarReserva(ctx, op, id, motivo, func(r domain.Reserva) (bool, error) {
		if a.Rol == RolAlumno && r.AlumnoID != a.ID {
			return false, domain.ErrSinPermiso // CA-15.4
		}
		return r.ValidarCancelacion(s.reloj.Ahora())
	})
}

// ---- Ingreso por DNI (HU-22, D-36 a D-39, D-82) ----

// RegistrarIngreso identifica al alumno por DNI, valida que esté activo y con
// membresía vigente hoy, y registra la asistencia de la reserva con la ventana
// abierta, o informa el reingreso sin puntos.
func (s *Servicio) RegistrarIngreso(ctx context.Context, a Actor, clave, dni string) (domain.Ingreso, bool, error) {
	if err := s.admin(a); err != nil {
		return domain.Ingreso{}, false, err
	}
	if !patronClave.MatchString(clave) || !patronDNI.MatchString(dni) {
		return domain.Ingreso{}, false, domain.ErrValidacion
	}
	op := ports.Operacion{Actor: a.ID, Clave: clave, Tipo: "INGRESO", Huella: huella("INGRESO", dni)}
	var previo domain.Ingreso
	if ok, err := s.reservas.Previa(ctx, op, &previo); err != nil || ok {
		return previo, ok, err
	}
	persona, err := s.members.AlumnoPorDNI(ctx, dni)
	if err != nil {
		// CA-22.7: un DNI inexistente no revela datos de nadie.
		return domain.Ingreso{}, false, dependencia(err)
	}
	nombre := persona.NombreCompleto()
	if persona.Estado != "ACTIVO" {
		return domain.Ingreso{}, false, RechazoIngreso{Motivo: domain.ErrUsuarioInactivo, Nombre: nombre}
	}
	vigencia, err := s.members.Vigencia(ctx, persona.ID, nil)
	if err != nil {
		return domain.Ingreso{}, false, dependencia(err)
	}
	if !vigencia.AlumnoActivo {
		return domain.Ingreso{}, false, RechazoIngreso{Motivo: domain.ErrUsuarioInactivo, Nombre: nombre}
	}
	if !vigencia.VigenteHoy {
		return domain.Ingreso{}, false, RechazoIngreso{Motivo: domain.ErrMembresiaNoVigente, Nombre: nombre}
	}
	now := s.reloj.Ahora()
	return s.reservas.RegistrarIngreso(ctx, op, persona.ID, now, func(reservas []domain.Reserva) (domain.Ingreso, error) {
		r, reingreso, err := domain.ElegirReservaParaIngreso(reservas, now)
		if err != nil {
			return domain.Ingreso{}, RechazoIngreso{Motivo: err, Nombre: nombre}
		}
		return domain.Ingreso{Reserva: r, Reingreso: reingreso, Nombre: nombre, Vence: vigencia.VenceHoy}, nil
	})
}

// CerrarClases es el proceso automático que finaliza clases y registra ausencias
// (RN-38, CA-22.8). Lo invoca el cableado de cmd/api periódicamente.
func (s *Servicio) CerrarClases(ctx context.Context) (int, int, error) {
	return s.reservas.CerrarClases(ctx, s.reloj.Ahora())
}
