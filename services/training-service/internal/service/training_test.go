// Etapa 10 de la entrega final: reglas de training con dobles en memoria.
// HU-17, HU-18, HU-23 a HU-25, HU-33, HU-34; RN-28 a RN-31 y RN-37. La
// persistencia y la concurrencia se prueban contra MongoDB en repository.
package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/GuadiViti/Arquitectura-de-Software/services/training-service/internal/model"
)

const (
	alumno    = "00000000-0000-4000-8000-000000000001"
	otro      = "00000000-0000-4000-8000-000000000002"
	profesor  = "00000000-0000-4000-8000-000000000010"
	profesor2 = "00000000-0000-4000-8000-000000000011"
	nutri     = "00000000-0000-4000-8000-000000000020"
	nutri2    = "00000000-0000-4000-8000-000000000021"
)

type reloj struct{ t time.Time }

func (r *reloj) Now() time.Time { return r.t }

type secuencia struct{ n int }

func (s *secuencia) NewID() (string, error) {
	s.n++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", 1000+s.n), nil
}

type membersFalso struct {
	asignaciones map[[2]string]string // (profesional, alumno) → rol
	caido        bool
}

func (m *membersFalso) AsignacionVigente(_ context.Context, p, a string) (bool, error) {
	if m.caido {
		return false, errors.New("members caído")
	}
	_, ok := m.asignaciones[[2]string{p, a}]
	return ok, nil
}
func (m *membersFalso) ProfesionalesDe(_ context.Context, a string) ([]model.Asignacion, error) {
	if m.caido {
		return nil, errors.New("members caído")
	}
	out := []model.Asignacion{}
	for k, rol := range m.asignaciones {
		if k[1] == a {
			out = append(out, model.Asignacion{ProfesionalID: k[0], AlumnoID: a, ProfesionalRol: rol})
		}
	}
	return out, nil
}
func (m *membersFalso) AlumnosDe(_ context.Context, p string) ([]model.Asignacion, error) {
	if m.caido {
		return nil, errors.New("members caído")
	}
	out := []model.Asignacion{}
	for k := range m.asignaciones {
		if k[0] == p {
			out = append(out, model.Asignacion{ProfesionalID: p, AlumnoID: k[1]})
		}
	}
	return out, nil
}

type repoFalso struct {
	entrenamiento []model.PlanEntrenamiento
	alimenticio   []model.PlanAlimenticio
	mediciones    []model.Medicion
	consultas     []model.Consulta
	escrituras    int
}

func (r *repoFalso) CrearPlanEntrenamiento(_ context.Context, p model.PlanEntrenamiento, ahora time.Time) error {
	r.escrituras++
	for i := range r.entrenamiento {
		if r.entrenamiento[i].AlumnoID == p.AlumnoID && r.entrenamiento[i].Estado == model.Vigente {
			r.entrenamiento[i].Estado, r.entrenamiento[i].FinalizadoEn = model.Finalizado, &ahora
		}
	}
	r.entrenamiento = append(r.entrenamiento, p)
	return nil
}
func (r *repoFalso) PlanesEntrenamiento(_ context.Context, a string) ([]model.PlanEntrenamiento, error) {
	out := []model.PlanEntrenamiento{}
	for i := len(r.entrenamiento) - 1; i >= 0; i-- {
		if r.entrenamiento[i].AlumnoID == a {
			out = append(out, r.entrenamiento[i])
		}
	}
	return out, nil
}
func (r *repoFalso) CrearPlanAlimenticio(_ context.Context, p model.PlanAlimenticio, ahora time.Time) error {
	r.escrituras++
	for i := range r.alimenticio {
		if r.alimenticio[i].AlumnoID == p.AlumnoID && r.alimenticio[i].Estado == model.Vigente {
			r.alimenticio[i].Estado, r.alimenticio[i].FinalizadoEn = model.Finalizado, &ahora
		}
	}
	r.alimenticio = append(r.alimenticio, p)
	return nil
}
func (r *repoFalso) PlanesAlimenticios(_ context.Context, a string) ([]model.PlanAlimenticio, error) {
	out := []model.PlanAlimenticio{}
	for i := len(r.alimenticio) - 1; i >= 0; i-- {
		if r.alimenticio[i].AlumnoID == a {
			out = append(out, r.alimenticio[i])
		}
	}
	return out, nil
}
func (r *repoFalso) RegistrarMedicion(_ context.Context, m model.Medicion) error {
	r.escrituras++
	r.mediciones = append(r.mediciones, m)
	return nil
}
func (r *repoFalso) Mediciones(_ context.Context, a string) ([]model.Medicion, error) {
	out := []model.Medicion{}
	for _, m := range r.mediciones {
		if m.AlumnoID == a {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Fecha < out[j].Fecha })
	return out, nil
}
func (r *repoFalso) CrearConsulta(_ context.Context, c model.Consulta) error {
	r.escrituras++
	r.consultas = append(r.consultas, c)
	return nil
}
func (r *repoFalso) ConsultasDeAlumnos(_ context.Context, alumnos []string, desc bool) ([]model.Consulta, error) {
	out := []model.Consulta{}
	for _, c := range r.consultas {
		for _, a := range alumnos {
			if c.AlumnoID == a {
				out = append(out, c)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if desc {
			return out[i].EnviadaEn.After(out[j].EnviadaEn)
		}
		return out[i].EnviadaEn.Before(out[j].EnviadaEn)
	})
	return out, nil
}
func (r *repoFalso) Consulta(_ context.Context, id string) (model.Consulta, error) {
	for _, c := range r.consultas {
		if c.ID == id {
			return c, nil
		}
	}
	return model.Consulta{}, model.ErrNoEncontrado
}
func (r *repoFalso) Responder(_ context.Context, id, n, resp string, ahora time.Time) (model.Consulta, error) {
	for i := range r.consultas {
		if r.consultas[i].ID == id {
			if r.consultas[i].Estado != model.Pendiente {
				return model.Consulta{}, model.ErrYaRespondida
			}
			r.escrituras++
			r.consultas[i].Estado, r.consultas[i].Respuesta, r.consultas[i].RespondidaEn, r.consultas[i].NutricionistaID = model.Respondida, &resp, &ahora, &n
			return r.consultas[i], nil
		}
	}
	return model.Consulta{}, model.ErrNoEncontrado
}

type escena struct {
	s       *Training
	repo    *repoFalso
	members *membersFalso
	reloj   *reloj
}

func nuevaEscena() escena {
	zona, _ := time.LoadLocation("America/Argentina/Buenos_Aires")
	e := escena{repo: &repoFalso{}, reloj: &reloj{time.Date(2026, 11, 10, 22, 30, 0, 0, zona)},
		members: &membersFalso{asignaciones: map[[2]string]string{{profesor, alumno}: model.RolProfesor, {nutri, alumno}: model.RolNutricionista}}}
	e.s = NewTraining(e.repo, e.members, e.reloj, &secuencia{}, zona)
	return e
}

func plan(desde, hasta string) model.PlanEntrenamiento {
	return model.PlanEntrenamiento{AlumnoID: alumno, Objetivo: "Hipertrofia", VigenciaDesde: desde, VigenciaHasta: hasta,
		Dias: []model.DiaRutina{{Nombre: "Día A", Ejercicios: []model.Ejercicio{{Nombre: "Sentadilla", GrupoMuscular: "Piernas", Series: 4, Repeticiones: 10, PesoSugerido: 40, DescansoSegundos: 90}}}}}
}

var (
	elAlumno   = model.Actor{ID: alumno, Rol: model.RolAlumno}
	elProfesor = model.Actor{ID: profesor, Rol: model.RolProfesor}
	elNutri    = model.Actor{ID: nutri, Rol: model.RolNutricionista}
)

func TestPlanEntrenamiento(t *testing.T) {
	ctx := context.Background()
	e := nuevaEscena()

	t.Run("CA-17.2_sin_plan", func(t *testing.T) {
		_, err := e.s.MiPlanEntrenamiento(ctx, elAlumno)
		require.ErrorIs(t, err, model.ErrNoEncontrado)
	})
	t.Run("CA-23.1_y_CA-17.1_crea_vigente_y_el_alumno_lo_ve", func(t *testing.T) {
		p, err := e.s.CrearPlanEntrenamiento(ctx, elProfesor, plan("2026-11-01", "2026-12-31"))
		require.NoError(t, err)
		require.Equal(t, model.Vigente, p.Estado)
		require.Equal(t, profesor, p.ProfesorID)
		mio, err := e.s.MiPlanEntrenamiento(ctx, elAlumno)
		require.NoError(t, err)
		require.Equal(t, "Sentadilla", mio.Dias[0].Ejercicios[0].Nombre)
	})
	t.Run("CA-23.2_nuevo_finaliza_el_anterior", func(t *testing.T) {
		_, err := e.s.CrearPlanEntrenamiento(ctx, elProfesor, plan("2026-11-10", "2027-01-10"))
		require.NoError(t, err)
		planes, err := e.s.PlanesEntrenamientoDe(ctx, elProfesor, alumno)
		require.NoError(t, err)
		require.Equal(t, "2026-11-10", planes.Vigente.VigenciaDesde)
		require.Len(t, planes.Historial, 1)
		require.Equal(t, model.Finalizado, planes.Historial[0].Estado)
	})
	t.Run("CA-23.3_y_RN-28_validaciones", func(t *testing.T) {
		antes := e.repo.escrituras
		for name, mut := range map[string]func(*model.PlanEntrenamiento){
			"series_cero":       func(p *model.PlanEntrenamiento) { p.Dias[0].Ejercicios[0].Series = 0 },
			"repeticiones_neg":  func(p *model.PlanEntrenamiento) { p.Dias[0].Ejercicios[0].Repeticiones = -1 },
			"peso_negativo":     func(p *model.PlanEntrenamiento) { p.Dias[0].Ejercicios[0].PesoSugerido = -1 },
			"descanso_negativo": func(p *model.PlanEntrenamiento) { p.Dias[0].Ejercicios[0].DescansoSegundos = -1 },
			"sin_objetivo":      func(p *model.PlanEntrenamiento) { p.Objetivo = "  " },
			"desde_mayor":       func(p *model.PlanEntrenamiento) { p.VigenciaDesde = "2027-02-01" },
			"fecha_invalida":    func(p *model.PlanEntrenamiento) { p.VigenciaHasta = "2026-13-01" },
			"sin_dias":          func(p *model.PlanEntrenamiento) { p.Dias = nil },
			"dia_sin_ejercicio": func(p *model.PlanEntrenamiento) { p.Dias[0].Ejercicios = nil },
		} {
			p := plan("2026-11-10", "2027-01-10")
			mut(&p)
			_, err := e.s.CrearPlanEntrenamiento(ctx, elProfesor, p)
			require.ErrorIs(t, err, model.ErrValidacion, name)
		}
		require.Equal(t, antes, e.repo.escrituras)
	})
	t.Run("CA-23.4_CA-23.5_y_RN-30_autorizacion", func(t *testing.T) {
		antes := e.repo.escrituras
		_, err := e.s.CrearPlanEntrenamiento(ctx, model.Actor{ID: profesor2, Rol: model.RolProfesor}, plan("2026-11-10", "2027-01-10"))
		require.ErrorIs(t, err, model.ErrSinPermiso, "profesor no asignado")
		_, err = e.s.CrearPlanEntrenamiento(ctx, elNutri, plan("2026-11-10", "2027-01-10"))
		require.ErrorIs(t, err, model.ErrSinPermiso, "un nutricionista asignado no crea planes de entrenamiento")
		_, err = e.s.PlanesEntrenamientoDe(ctx, model.Actor{ID: profesor2, Rol: model.RolProfesor}, alumno)
		require.ErrorIs(t, err, model.ErrSinPermiso)
		_, err = e.s.MiPlanEntrenamiento(ctx, elProfesor)
		require.ErrorIs(t, err, model.ErrSinPermiso)
		delete(e.members.asignaciones, [2]string{profesor, alumno})
		_, err = e.s.CrearPlanEntrenamiento(ctx, elProfesor, plan("2026-11-10", "2027-01-10"))
		require.ErrorIs(t, err, model.ErrSinPermiso, "revocar impide la siguiente operación, sin período de gracia")
		e.members.asignaciones[[2]string{profesor, alumno}] = model.RolProfesor
		e.members.caido = true
		_, err = e.s.CrearPlanEntrenamiento(ctx, elProfesor, plan("2026-11-10", "2027-01-10"))
		require.ErrorIs(t, err, model.ErrNoDisponible, "members caído: falla cerrada")
		e.members.caido = false
		require.Equal(t, antes, e.repo.escrituras, "ningún rechazo modifica datos")
	})
	t.Run("vigencia_vencida_no_es_vigente", func(t *testing.T) {
		e.reloj.t = e.reloj.t.AddDate(0, 3, 0)
		_, err := e.s.MiPlanEntrenamiento(ctx, elAlumno)
		require.ErrorIs(t, err, model.ErrNoEncontrado)
		e.reloj.t = e.reloj.t.AddDate(0, -3, 0)
	})
}

func TestNutricion(t *testing.T) {
	ctx := context.Background()
	e := nuevaEscena()
	masa := 30.0

	t.Run("CA-24.1_y_CA-18.1_plan_alimenticio", func(t *testing.T) {
		_, err := e.s.CrearPlanAlimenticio(ctx, elNutri, model.PlanAlimenticio{AlumnoID: alumno, Observaciones: "Solo observaciones"})
		require.ErrorIs(t, err, model.ErrValidacion, "al menos una comida")
		_, err = e.s.CrearPlanAlimenticio(ctx, elNutri, model.PlanAlimenticio{AlumnoID: alumno, Desayuno: "Avena", Cena: "Pescado"})
		require.NoError(t, err)
		p, err := e.s.CrearPlanAlimenticio(ctx, elNutri, model.PlanAlimenticio{AlumnoID: alumno, Almuerzo: "Pollo"})
		require.NoError(t, err)
		require.Equal(t, "2026-11-10", p.Fecha, "fecha en hora de Argentina")
		mio, err := e.s.MiPlanAlimenticio(ctx, elAlumno)
		require.NoError(t, err)
		require.Equal(t, "Pollo", mio.Almuerzo)
		planes, err := e.s.PlanesAlimenticiosDe(ctx, elNutri, alumno)
		require.NoError(t, err)
		require.Len(t, planes.Historial, 1)
	})
	t.Run("CA-24.2_profesor_no_carga_plan_alimenticio", func(t *testing.T) {
		_, err := e.s.CrearPlanAlimenticio(ctx, elProfesor, model.PlanAlimenticio{AlumnoID: alumno, Desayuno: "Avena"})
		require.ErrorIs(t, err, model.ErrSinPermiso)
	})
	t.Run("CA-25.1_CA-25.3_CA-25.4_RN-31_mediciones", func(t *testing.T) {
		valida := model.Medicion{AlumnoID: alumno, Fecha: "2026-11-10", Peso: 70, PorcentajeGrasa: 20, PorcentajeMuscular: 40, MasaMuscular: &masa}
		_, err := e.s.RegistrarMedicion(ctx, elNutri, valida)
		require.NoError(t, err)
		vieja := valida
		vieja.Fecha = "2026-10-01"
		_, err = e.s.RegistrarMedicion(ctx, elNutri, vieja)
		require.NoError(t, err)
		_, err = e.s.RegistrarMedicion(ctx, elNutri, valida)
		require.NoError(t, err, "puede haber más de una medición en la misma fecha")
		masaGrande := 80.0
		for name, mut := range map[string]func(*model.Medicion){
			"grasa_120":      func(m *model.Medicion) { m.PorcentajeGrasa = 120 },
			"peso_cero":      func(m *model.Medicion) { m.Peso = 0 },
			"futura":         func(m *model.Medicion) { m.Fecha = "2026-11-11" },
			"suma_mayor":     func(m *model.Medicion) { m.PorcentajeGrasa, m.PorcentajeMuscular = 60, 50 },
			"masa_ge_peso":   func(m *model.Medicion) { m.MasaMuscular = &masaGrande },
			"fecha_invalida": func(m *model.Medicion) { m.Fecha = "10/11/2026" },
		} {
			m := valida
			mut(&m)
			_, err := e.s.RegistrarMedicion(ctx, elNutri, m)
			require.ErrorIs(t, err, model.ErrValidacion, name)
		}
		mias, err := e.s.MisMediciones(ctx, elAlumno)
		require.NoError(t, err)
		require.Len(t, mias, 3)
		require.Equal(t, "2026-10-01", mias[0].Fecha, "CA-18.2 ordenadas por fecha")
		_, err = e.s.RegistrarMedicion(ctx, elProfesor, valida)
		require.ErrorIs(t, err, model.ErrSinPermiso)
	})
}

func TestConsultas(t *testing.T) {
	ctx := context.Background()
	e := nuevaEscena()

	t.Run("CA-33.2_sin_nutricionista", func(t *testing.T) {
		_, err := e.s.EnviarConsulta(ctx, model.Actor{ID: otro, Rol: model.RolAlumno}, "¿Puedo comer fruta?")
		require.ErrorIs(t, err, model.ErrSinNutricionista)
	})
	t.Run("CA-33.4_texto_vacio", func(t *testing.T) {
		_, err := e.s.EnviarConsulta(ctx, elAlumno, "   ")
		require.ErrorIs(t, err, model.ErrValidacion)
	})
	var primera model.Consulta
	t.Run("CA-33.1_CA-34.1_pendiente_y_bandeja_ordenada", func(t *testing.T) {
		var err error
		primera, err = e.s.EnviarConsulta(ctx, elAlumno, "¿Puedo reemplazar la colación por fruta?")
		require.NoError(t, err)
		require.Equal(t, model.Pendiente, primera.Estado)
		e.reloj.t = e.reloj.t.Add(time.Minute)
		_, err = e.s.EnviarConsulta(ctx, elAlumno, "¿Y la cena?")
		require.NoError(t, err)
		bandeja, err := e.s.Bandeja(ctx, elNutri)
		require.NoError(t, err)
		require.Len(t, bandeja, 2)
		require.Equal(t, primera.ID, bandeja[0].ID, "de la más antigua a la más reciente")
	})
	t.Run("CA-34.3_otro_nutricionista_no_ve_ni_responde", func(t *testing.T) {
		otroNutri := model.Actor{ID: nutri2, Rol: model.RolNutricionista}
		bandeja, err := e.s.Bandeja(ctx, otroNutri)
		require.NoError(t, err)
		require.Empty(t, bandeja)
		_, err = e.s.Responder(ctx, otroNutri, primera.ID, "Sí")
		require.ErrorIs(t, err, model.ErrSinPermiso)
	})
	t.Run("CA-34.2_CA-34.4_CA-33.3_responder_una_vez", func(t *testing.T) {
		r, err := e.s.Responder(ctx, elNutri, primera.ID, "Sí, una fruta de estación.")
		require.NoError(t, err)
		require.Equal(t, model.Respondida, r.Estado)
		_, err = e.s.Responder(ctx, elNutri, primera.ID, "Otra respuesta")
		require.ErrorIs(t, err, model.ErrYaRespondida)
		mias, err := e.s.MisConsultas(ctx, elAlumno)
		require.NoError(t, err)
		respondida := mias[1]
		require.Equal(t, "Sí, una fruta de estación.", *respondida.Respuesta)
		require.Equal(t, nutri, *respondida.NutricionistaID)
		require.NotNil(t, respondida.RespondidaEn)
	})
	t.Run("RN-37_pendientes_pasan_al_nuevo_nutricionista", func(t *testing.T) {
		delete(e.members.asignaciones, [2]string{nutri, alumno})
		e.members.asignaciones[[2]string{nutri2, alumno}] = model.RolNutricionista
		bandeja, err := e.s.Bandeja(ctx, model.Actor{ID: nutri2, Rol: model.RolNutricionista})
		require.NoError(t, err)
		pendientes := 0
		for _, c := range bandeja {
			if c.Estado == model.Pendiente {
				pendientes++
			}
		}
		require.Equal(t, 1, pendientes)
		viejo, err := e.s.Bandeja(ctx, elNutri)
		require.NoError(t, err)
		require.Empty(t, viejo, "el nutricionista anterior ya no las ve")
	})
}
