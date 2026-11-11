// Etapa 10 contra MongoDB 7 real (testcontainers): un solo plan VIGENTE por
// alumno garantizado por índice (RN-28/29) aun con altas concurrentes,
// validadores de mediciones (RN-31) y respuesta única concurrente (CA-34.4).
package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/GuadiViti/Arquitectura-de-Software/services/training-service/internal/model"
)

func TestTrainingMongoReal(t *testing.T) {
	requireDocker(t)
	ctx := context.Background()
	ctr, err := mongodb.Run(ctx, "mongo:7")
	t.Cleanup(func() {
		if ctr != nil {
			_ = ctr.Terminate(context.Background())
		}
	})
	require.NoError(t, err)
	uri, err := ctr.ConnectionString(ctx)
	require.NoError(t, err)
	m, err := NewMongo(uri, "training_db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close(context.Background()) })
	require.NoError(t, m.EnsureSchema(ctx))
	require.NoError(t, m.EnsureSchema(ctx), "idempotente")
	ahora := time.Now().UTC().Truncate(time.Millisecond)
	plan := func(n int, alumno string) model.PlanEntrenamiento {
		return model.PlanEntrenamiento{ID: fmt.Sprintf("plan-%d", n), AlumnoID: alumno, ProfesorID: "prof", Objetivo: "Fuerza", VigenciaDesde: "2026-11-10", VigenciaHasta: "2026-12-10",
			Dias: []model.DiaRutina{{Nombre: "A", Ejercicios: []model.Ejercicio{{Nombre: "Press", Series: 3, Repeticiones: 8}}}}, Estado: model.Vigente, CreadoEn: ahora.Add(time.Duration(n) * time.Millisecond)}
	}
	vigentes := func(alumno string) int64 {
		n, err := m.db.Collection(colEntrenamiento).CountDocuments(ctx, bson.M{"alumno_id": alumno, "estado": model.Vigente})
		require.NoError(t, err)
		return n
	}

	t.Run("RN-28_reemplazo_conserva_historial", func(t *testing.T) {
		require.NoError(t, m.CrearPlanEntrenamiento(ctx, plan(1, "a1"), ahora))
		require.NoError(t, m.CrearPlanEntrenamiento(ctx, plan(2, "a1"), ahora))
		planes, err := m.PlanesEntrenamiento(ctx, "a1")
		require.NoError(t, err)
		require.Len(t, planes, 2)
		require.Equal(t, "plan-2", planes[0].ID, "más reciente primero")
		require.Equal(t, model.Finalizado, planes[1].Estado)
		require.NotNil(t, planes[1].FinalizadoEn)
		require.EqualValues(t, 1, vigentes("a1"))
	})
	t.Run("indice_rechaza_dos_vigentes_directos", func(t *testing.T) {
		_, err := m.db.Collection(colEntrenamiento).InsertOne(ctx, plan(3, "a1"))
		require.Error(t, err, "la base garantiza un solo VIGENTE aunque se esquive la capa service")
	})
	t.Run("altas_concurrentes_dejan_un_solo_vigente", func(t *testing.T) {
		var wg sync.WaitGroup
		errs := make(chan error, 10)
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(i int) { defer wg.Done(); errs <- m.CrearPlanEntrenamiento(ctx, plan(100+i, "a2"), ahora) }(i)
		}
		wg.Wait()
		close(errs)
		ok := 0
		for err := range errs {
			if err == nil {
				ok++
			}
		}
		require.GreaterOrEqual(t, ok, 1)
		require.EqualValues(t, 1, vigentes("a2"))
	})
	t.Run("RN-29_plan_alimenticio_un_vigente", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			require.NoError(t, m.CrearPlanAlimenticio(ctx, model.PlanAlimenticio{ID: fmt.Sprintf("alim-%d", i), AlumnoID: "a1", NutricionistaID: "n", Fecha: "2026-11-10", Desayuno: "Avena", Estado: model.Vigente, CreadoEn: ahora.Add(time.Duration(i) * time.Second)}, ahora))
		}
		planes, err := m.PlanesAlimenticios(ctx, "a1")
		require.NoError(t, err)
		require.Equal(t, model.Vigente, planes[0].Estado)
		require.Equal(t, model.Finalizado, planes[1].Estado)
	})
	t.Run("RN-31_validador_y_orden_de_mediciones", func(t *testing.T) {
		masa := 30.0
		for i, f := range []string{"2026-11-10", "2026-10-01", "2026-11-10"} {
			require.NoError(t, m.RegistrarMedicion(ctx, model.Medicion{ID: fmt.Sprintf("med-%d", i), AlumnoID: "a1", NutricionistaID: "n", Fecha: f, Peso: 70, PorcentajeGrasa: 20, PorcentajeMuscular: 40, MasaMuscular: &masa, RegistradaEn: ahora.Add(time.Duration(i) * time.Second)}))
		}
		meds, err := m.Mediciones(ctx, "a1")
		require.NoError(t, err)
		require.Equal(t, []string{"med-1", "med-0", "med-2"}, []string{meds[0].ID, meds[1].ID, meds[2].ID})
		for name, doc := range map[string]bson.M{
			"grasa_120": {"_id": "x1", "alumno_id": "a1", "nutricionista_id": "n", "fecha": "2026-11-10", "peso": 70.0, "porcentaje_grasa": 120.0, "porcentaje_muscular": 10.0, "registrada_en": ahora},
			"peso_cero": {"_id": "x2", "alumno_id": "a1", "nutricionista_id": "n", "fecha": "2026-11-10", "peso": 0.0, "porcentaje_grasa": 10.0, "porcentaje_muscular": 10.0, "registrada_en": ahora},
			"sin_peso":  {"_id": "x3", "alumno_id": "a1", "nutricionista_id": "n", "fecha": "2026-11-10", "porcentaje_grasa": 10.0, "porcentaje_muscular": 10.0, "registrada_en": ahora},
		} {
			_, err := m.db.Collection(colMediciones).InsertOne(ctx, doc)
			require.Error(t, err, name)
		}
	})
	t.Run("CA-34.4_respuesta_unica_concurrente", func(t *testing.T) {
		require.NoError(t, m.CrearConsulta(ctx, model.Consulta{ID: "c1", AlumnoID: "a1", Pregunta: "¿Fruta?", EnviadaEn: ahora, Estado: model.Pendiente}))
		require.NoError(t, m.CrearConsulta(ctx, model.Consulta{ID: "c0", AlumnoID: "a1", Pregunta: "¿Antes?", EnviadaEn: ahora.Add(-time.Minute), Estado: model.Pendiente}))
		var wg sync.WaitGroup
		resultados := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, err := m.Responder(ctx, "c1", "n", fmt.Sprintf("respuesta %d", i), ahora)
				resultados <- err
			}(i)
		}
		wg.Wait()
		close(resultados)
		ok := 0
		for err := range resultados {
			if err == nil {
				ok++
			} else {
				require.ErrorIs(t, err, model.ErrYaRespondida)
			}
		}
		require.Equal(t, 1, ok)
		_, err := m.Responder(ctx, "no-existe", "n", "x", ahora)
		require.ErrorIs(t, err, model.ErrNoEncontrado)
		asc, err := m.ConsultasDeAlumnos(ctx, []string{"a1"}, false)
		require.NoError(t, err)
		require.Equal(t, "c0", asc[0].ID)
		_, err = m.db.Collection(colConsultas).InsertOne(ctx, bson.M{"_id": "c9", "alumno_id": "a1", "pregunta": "x", "enviada_en": ahora, "estado": "BORRADOR"})
		require.Error(t, err, "estado fuera del ciclo de vida")
	})
}
