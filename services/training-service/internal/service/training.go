package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GuadiViti/Arquitectura-de-Software/services/training-service/internal/model"
)

// Repository es lo que la capa service necesita de la persistencia (training_db).
type Repository interface {
	CrearPlanEntrenamiento(ctx context.Context, p model.PlanEntrenamiento, ahora time.Time) error
	PlanesEntrenamiento(ctx context.Context, alumno string) ([]model.PlanEntrenamiento, error)
	CrearPlanAlimenticio(ctx context.Context, p model.PlanAlimenticio, ahora time.Time) error
	PlanesAlimenticios(ctx context.Context, alumno string) ([]model.PlanAlimenticio, error)
	RegistrarMedicion(ctx context.Context, m model.Medicion) error
	Mediciones(ctx context.Context, alumno string) ([]model.Medicion, error)
	CrearConsulta(ctx context.Context, c model.Consulta) error
	ConsultasDeAlumnos(ctx context.Context, alumnos []string, masRecientePrimero bool) ([]model.Consulta, error)
	Consulta(ctx context.Context, id string) (model.Consulta, error)
	// Responder pasa PENDIENTE → RESPONDIDA en una sola operación condicional.
	Responder(ctx context.Context, id, nutricionista, respuesta string, ahora time.Time) (model.Consulta, error)
}

// Members confirma asignaciones vigentes en cada operación, sin caché (D-33).
type Members interface {
	AsignacionVigente(ctx context.Context, profesional, alumno string) (bool, error)
	ProfesionalesDe(ctx context.Context, alumno string) ([]model.Asignacion, error)
	AlumnosDe(ctx context.Context, profesional string) ([]model.Asignacion, error)
}

// Clock e IDs se inyectan para los tests.
type Clock interface{ Now() time.Time }
type IDs interface{ NewID() (string, error) }

// Training implementa HU-17, HU-18, HU-23 a HU-25, HU-33 y HU-34.
type Training struct {
	repo    Repository
	members Members
	clock   Clock
	ids     IDs
	zona    *time.Location
}

func NewTraining(repo Repository, members Members, clock Clock, ids IDs, zona *time.Location) *Training {
	return &Training{repo: repo, members: members, clock: clock, ids: ids, zona: zona}
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (t *Training) hoy() string { return t.clock.Now().In(t.zona).Format(model.FechaLayout) }

// dependencia marca como 503 una falla técnica sin ocultar errores de negocio.
func dependencia(err error) error {
	if err == nil || errors.Is(err, model.ErrNoEncontrado) || errors.Is(err, model.ErrYaRespondida) || errors.Is(err, model.ErrValidacion) {
		return err
	}
	return errors.Join(model.ErrNoDisponible, err)
}

func texto(s string, min, max int) (string, bool) {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	return s, utf8.ValidString(s) && !strings.ContainsRune(s, 0) && n >= min && n <= max
}

func fecha(s string) bool {
	_, err := time.Parse(model.FechaLayout, s)
	return err == nil
}

func propio(a model.Actor, rol string) error {
	if a.Rol != rol || !uuidPattern.MatchString(a.ID) {
		return model.ErrSinPermiso
	}
	return nil
}

// asignado exige el subtipo correcto (CA-23.5, CA-24.2) y una asignación vigente
// confirmada por members en este momento (RN-30). Si members no responde, falla
// cerrada sin modificar datos (ADR-007).
func (t *Training) asignado(ctx context.Context, a model.Actor, rol, alumno string) error {
	if err := propio(a, rol); err != nil {
		return err
	}
	if !uuidPattern.MatchString(alumno) {
		return model.ErrValidacion
	}
	ok, err := t.members.AsignacionVigente(ctx, a.ID, alumno)
	if err != nil {
		return errors.Join(model.ErrNoDisponible, err)
	}
	if !ok {
		return model.ErrSinPermiso
	}
	return nil
}

// ---- Entrenamiento (RN-28) ----

// efectivo finaliza en la respuesta un plan cuya vigencia ya terminó (SPEC §7).
func (t *Training) efectivo(p model.PlanEntrenamiento) model.PlanEntrenamiento {
	if p.Estado == model.Vigente && p.VigenciaHasta < t.hoy() {
		p.Estado = model.Finalizado
	}
	return p
}

func (t *Training) planesEntrenamiento(ctx context.Context, alumno string) (model.PlanesEntrenamiento, error) {
	planes, err := t.repo.PlanesEntrenamiento(ctx, alumno)
	if err != nil {
		return model.PlanesEntrenamiento{}, dependencia(err)
	}
	out := model.PlanesEntrenamiento{Historial: []model.PlanEntrenamiento{}}
	for _, p := range planes {
		p = t.efectivo(p)
		if p.Estado == model.Vigente && out.Vigente == nil {
			copia := p
			out.Vigente = &copia
			continue
		}
		out.Historial = append(out.Historial, p)
	}
	return out, nil
}

// MiPlanEntrenamiento devuelve el plan vigente del alumno (HU-17); sin plan, 404 (CA-17.2).
func (t *Training) MiPlanEntrenamiento(ctx context.Context, a model.Actor) (model.PlanEntrenamiento, error) {
	if err := propio(a, model.RolAlumno); err != nil {
		return model.PlanEntrenamiento{}, err
	}
	planes, err := t.planesEntrenamiento(ctx, a.ID)
	if err != nil {
		return model.PlanEntrenamiento{}, err
	}
	if planes.Vigente == nil {
		return model.PlanEntrenamiento{}, model.ErrNoEncontrado
	}
	return *planes.Vigente, nil
}

// PlanesEntrenamientoDe muestra vigente e historial al profesor asignado.
func (t *Training) PlanesEntrenamientoDe(ctx context.Context, a model.Actor, alumno string) (model.PlanesEntrenamiento, error) {
	if err := t.asignado(ctx, a, model.RolProfesor, alumno); err != nil {
		return model.PlanesEntrenamiento{}, err
	}
	return t.planesEntrenamiento(ctx, alumno)
}

func validarPlan(p model.PlanEntrenamiento) (model.PlanEntrenamiento, bool) {
	var ok bool
	if p.Objetivo, ok = texto(p.Objetivo, 1, 200); !ok || !fecha(p.VigenciaDesde) || !fecha(p.VigenciaHasta) || p.VigenciaDesde > p.VigenciaHasta {
		return p, false
	}
	if p.Observaciones, ok = texto(p.Observaciones, 0, 1000); !ok || len(p.Dias) < 1 || len(p.Dias) > 14 {
		return p, false
	}
	for i := range p.Dias {
		d := &p.Dias[i]
		if d.Nombre, ok = texto(d.Nombre, 1, 100); !ok {
			return p, false
		}
		if d.Descripcion, ok = texto(d.Descripcion, 0, 500); !ok || len(d.Ejercicios) < 1 || len(d.Ejercicios) > 50 {
			return p, false
		}
		for j := range d.Ejercicios {
			e := &d.Ejercicios[j]
			if e.Nombre, ok = texto(e.Nombre, 1, 100); !ok {
				return p, false
			}
			if e.GrupoMuscular, ok = texto(e.GrupoMuscular, 0, 100); !ok {
				return p, false
			}
			if e.Observaciones, ok = texto(e.Observaciones, 0, 500); !ok {
				return p, false
			}
			// CA-23.3: series y repeticiones > 0; peso y descanso ≥ 0 (RN-28).
			if e.Series < 1 || e.Series > 100 || e.Repeticiones < 1 || e.Repeticiones > 1000 || e.PesoSugerido < 0 || e.PesoSugerido > 1000 || e.DescansoSegundos < 0 || e.DescansoSegundos > 3600 {
				return p, false
			}
		}
	}
	return p, true
}

// CrearPlanEntrenamiento (HU-23): solo el PROFESOR asignado; el plan vigente
// anterior pasa a FINALIZADO y se conserva (CA-23.1/2).
func (t *Training) CrearPlanEntrenamiento(ctx context.Context, a model.Actor, p model.PlanEntrenamiento) (model.PlanEntrenamiento, error) {
	if err := t.asignado(ctx, a, model.RolProfesor, p.AlumnoID); err != nil {
		return model.PlanEntrenamiento{}, err
	}
	p, ok := validarPlan(p)
	if !ok {
		return model.PlanEntrenamiento{}, model.ErrValidacion
	}
	id, err := t.ids.NewID()
	if err != nil {
		return model.PlanEntrenamiento{}, err
	}
	ahora := t.clock.Now().UTC().Truncate(time.Millisecond)
	p.ID, p.ProfesorID, p.Estado, p.CreadoEn, p.FinalizadoEn = id, a.ID, model.Vigente, ahora, nil
	p.AlumnoID = strings.ToLower(p.AlumnoID)
	if err := t.repo.CrearPlanEntrenamiento(ctx, p, ahora); err != nil {
		return model.PlanEntrenamiento{}, dependencia(err)
	}
	return t.efectivo(p), nil
}

// ---- Nutrición (RN-29, RN-31) ----

func (t *Training) planesAlimenticios(ctx context.Context, alumno string) (model.PlanesAlimenticios, error) {
	planes, err := t.repo.PlanesAlimenticios(ctx, alumno)
	if err != nil {
		return model.PlanesAlimenticios{}, dependencia(err)
	}
	out := model.PlanesAlimenticios{Historial: []model.PlanAlimenticio{}}
	for _, p := range planes {
		if p.Estado == model.Vigente && out.Vigente == nil {
			copia := p
			out.Vigente = &copia
			continue
		}
		out.Historial = append(out.Historial, p)
	}
	return out, nil
}

// MiPlanAlimenticio devuelve el plan vigente del alumno (CA-18.1).
func (t *Training) MiPlanAlimenticio(ctx context.Context, a model.Actor) (model.PlanAlimenticio, error) {
	if err := propio(a, model.RolAlumno); err != nil {
		return model.PlanAlimenticio{}, err
	}
	planes, err := t.planesAlimenticios(ctx, a.ID)
	if err != nil {
		return model.PlanAlimenticio{}, err
	}
	if planes.Vigente == nil {
		return model.PlanAlimenticio{}, model.ErrNoEncontrado
	}
	return *planes.Vigente, nil
}

// PlanesAlimenticiosDe muestra vigente e historial al nutricionista asignado.
func (t *Training) PlanesAlimenticiosDe(ctx context.Context, a model.Actor, alumno string) (model.PlanesAlimenticios, error) {
	if err := t.asignado(ctx, a, model.RolNutricionista, alumno); err != nil {
		return model.PlanesAlimenticios{}, err
	}
	return t.planesAlimenticios(ctx, alumno)
}

// CrearPlanAlimenticio (HU-24): al menos una comida; el anterior se finaliza.
func (t *Training) CrearPlanAlimenticio(ctx context.Context, a model.Actor, p model.PlanAlimenticio) (model.PlanAlimenticio, error) {
	if err := t.asignado(ctx, a, model.RolNutricionista, p.AlumnoID); err != nil {
		return model.PlanAlimenticio{}, err
	}
	comidas := 0
	for _, campo := range []*string{&p.Desayuno, &p.Colacion, &p.Almuerzo, &p.Merienda, &p.Cena, &p.Observaciones} {
		v, ok := texto(*campo, 0, 1000)
		if !ok {
			return model.PlanAlimenticio{}, model.ErrValidacion
		}
		*campo = v
		if v != "" && campo != &p.Observaciones {
			comidas++
		}
	}
	if comidas == 0 {
		return model.PlanAlimenticio{}, model.ErrValidacion
	}
	id, err := t.ids.NewID()
	if err != nil {
		return model.PlanAlimenticio{}, err
	}
	ahora := t.clock.Now().UTC().Truncate(time.Millisecond)
	p.ID, p.NutricionistaID, p.Fecha, p.Estado, p.CreadoEn, p.FinalizadoEn = id, a.ID, t.hoy(), model.Vigente, ahora, nil
	p.AlumnoID = strings.ToLower(p.AlumnoID)
	if err := t.repo.CrearPlanAlimenticio(ctx, p, ahora); err != nil {
		return model.PlanAlimenticio{}, dependencia(err)
	}
	return p, nil
}

// MisMediciones devuelve la evolución propia ordenada por fecha (CA-18.2).
func (t *Training) MisMediciones(ctx context.Context, a model.Actor) ([]model.Medicion, error) {
	if err := propio(a, model.RolAlumno); err != nil {
		return nil, err
	}
	out, err := t.repo.Mediciones(ctx, a.ID)
	return out, dependencia(err)
}

// MedicionesDe muestra el historial al nutricionista asignado.
func (t *Training) MedicionesDe(ctx context.Context, a model.Actor, alumno string) ([]model.Medicion, error) {
	if err := t.asignado(ctx, a, model.RolNutricionista, alumno); err != nil {
		return nil, err
	}
	out, err := t.repo.Mediciones(ctx, strings.ToLower(alumno))
	return out, dependencia(err)
}

// RegistrarMedicion (HU-25, RN-31): fecha ≤ hoy, peso > 0, porcentajes 0–100
// con suma ≤ 100 y masa muscular > 0 y < peso. Nunca se modifica ni se borra.
func (t *Training) RegistrarMedicion(ctx context.Context, a model.Actor, m model.Medicion) (model.Medicion, error) {
	if err := t.asignado(ctx, a, model.RolNutricionista, m.AlumnoID); err != nil {
		return model.Medicion{}, err
	}
	var ok bool
	if m.Observaciones, ok = texto(m.Observaciones, 0, 500); !ok || !fecha(m.Fecha) || m.Fecha > t.hoy() {
		return model.Medicion{}, model.ErrValidacion
	}
	if m.Peso <= 0 || m.Peso > 500 || m.PorcentajeGrasa < 0 || m.PorcentajeGrasa > 100 || m.PorcentajeMuscular < 0 || m.PorcentajeMuscular > 100 || m.PorcentajeGrasa+m.PorcentajeMuscular > 100 {
		return model.Medicion{}, model.ErrValidacion
	}
	if m.MasaMuscular != nil && (*m.MasaMuscular <= 0 || *m.MasaMuscular >= m.Peso) {
		return model.Medicion{}, model.ErrValidacion
	}
	id, err := t.ids.NewID()
	if err != nil {
		return model.Medicion{}, err
	}
	m.ID, m.NutricionistaID, m.RegistradaEn = id, a.ID, t.clock.Now().UTC().Truncate(time.Millisecond)
	m.AlumnoID = strings.ToLower(m.AlumnoID)
	if err := t.repo.RegistrarMedicion(ctx, m); err != nil {
		return model.Medicion{}, dependencia(err)
	}
	return m, nil
}

// ---- Consultas nutricionales (RN-37) ----

// EnviarConsulta (HU-33): solo con nutricionista asignado (CA-33.2); texto no vacío (CA-33.4).
func (t *Training) EnviarConsulta(ctx context.Context, a model.Actor, pregunta string) (model.Consulta, error) {
	if err := propio(a, model.RolAlumno); err != nil {
		return model.Consulta{}, err
	}
	pregunta, ok := texto(pregunta, 1, 1000)
	if !ok {
		return model.Consulta{}, model.ErrValidacion
	}
	profesionales, err := t.members.ProfesionalesDe(ctx, a.ID)
	if err != nil {
		return model.Consulta{}, errors.Join(model.ErrNoDisponible, err)
	}
	tiene := false
	for _, p := range profesionales {
		tiene = tiene || p.ProfesionalRol == model.RolNutricionista
	}
	if !tiene {
		return model.Consulta{}, model.ErrSinNutricionista
	}
	id, err := t.ids.NewID()
	if err != nil {
		return model.Consulta{}, err
	}
	c := model.Consulta{ID: id, AlumnoID: strings.ToLower(a.ID), Pregunta: pregunta, EnviadaEn: t.clock.Now().UTC().Truncate(time.Millisecond), Estado: model.Pendiente}
	if err := t.repo.CrearConsulta(ctx, c); err != nil {
		return model.Consulta{}, dependencia(err)
	}
	return c, nil
}

// MisConsultas: pregunta, respuesta, quién respondió y fechas (CA-33.3).
func (t *Training) MisConsultas(ctx context.Context, a model.Actor) ([]model.Consulta, error) {
	if err := propio(a, model.RolAlumno); err != nil {
		return nil, err
	}
	out, err := t.repo.ConsultasDeAlumnos(ctx, []string{strings.ToLower(a.ID)}, true)
	return out, dependencia(err)
}

// Bandeja lista las consultas de los pacientes actuales del nutricionista, de
// la más antigua a la más reciente (CA-34.1). Las PENDIENTE no están atadas a
// un nutricionista: si cambia la asignación las ve el nuevo (RN-37).
func (t *Training) Bandeja(ctx context.Context, a model.Actor) ([]model.Consulta, error) {
	if err := propio(a, model.RolNutricionista); err != nil {
		return nil, err
	}
	pacientes, err := t.members.AlumnosDe(ctx, a.ID)
	if err != nil {
		return nil, errors.Join(model.ErrNoDisponible, err)
	}
	ids := make([]string, 0, len(pacientes))
	for _, p := range pacientes {
		ids = append(ids, strings.ToLower(p.AlumnoID))
	}
	if len(ids) == 0 {
		return []model.Consulta{}, nil
	}
	out, err := t.repo.ConsultasDeAlumnos(ctx, ids, false)
	return out, dependencia(err)
}

// Responder (HU-34): solo el nutricionista asignado al paciente (CA-34.3), una
// sola vez (CA-34.4).
func (t *Training) Responder(ctx context.Context, a model.Actor, id, respuesta string) (model.Consulta, error) {
	if err := propio(a, model.RolNutricionista); err != nil {
		return model.Consulta{}, err
	}
	respuesta, ok := texto(respuesta, 1, 2000)
	if !ok || !uuidPattern.MatchString(id) {
		return model.Consulta{}, model.ErrValidacion
	}
	c, err := t.repo.Consulta(ctx, strings.ToLower(id))
	if err != nil {
		return model.Consulta{}, dependencia(err)
	}
	if err := t.asignado(ctx, a, model.RolNutricionista, c.AlumnoID); err != nil {
		return model.Consulta{}, err
	}
	out, err := t.repo.Responder(ctx, c.ID, a.ID, respuesta, t.clock.Now().UTC().Truncate(time.Millisecond))
	return out, dependencia(err)
}
