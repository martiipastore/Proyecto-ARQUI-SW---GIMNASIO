// Etapa 8 de la entrega final con PostgreSQL real: cada escritura de una clase
// incrementa su versión y deja clase.actualizada en el outbox en la misma
// transacción (ADR-014, CLAUDE.md regla 6); fuente para reindexar y marcas
// "ya reservada" (CA-13.4).
package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/domain"
)

func TestBusquedaPostgres(t *testing.T) {
	db := nuevaBase(t)
	ctx := context.Background()
	zumba := actividadPorNombre(t, db, "Zumba")
	inicio := time.Now().Add(48 * time.Hour).Truncate(time.Minute)

	eventos := func(id string) []map[string]any {
		rows, err := db.pool.Query(ctx, `SELECT data FROM outbox WHERE event_type='clase.actualizada' AND agregado_id=$1 ORDER BY (data->>'version')::bigint`, id)
		require.NoError(t, err)
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var raw []byte
			require.NoError(t, rows.Scan(&raw))
			var m map[string]any
			require.NoError(t, json.Unmarshal(raw, &m))
			out = append(out, m)
		}
		return out
	}

	c := clase(t, db, zumba, uuidN(900), inicio, 60, 2)
	require.EqualValues(t, 1, c.Version)

	t.Run("cada_escritura_versiona_y_publica_en_la_misma_transaccion", func(t *testing.T) {
		r, _, err := reservar(db, uuidN(901), "busqueda-reserva-01", c.ID)
		require.NoError(t, err)
		_, _, err = cancelar(db, uuidN(901), "busqueda-cancela-01", r.ID)
		require.NoError(t, err)
		evs := eventos(c.ID)
		require.Len(t, evs, 3, "alta, reserva y cancelación")
		for i, e := range evs {
			require.EqualValues(t, i+1, e["version"])
			require.Equal(t, "Zumba", e["actividad_nombre"])
		}
		require.EqualValues(t, 1, evs[1]["ocupacion"])
		require.EqualValues(t, 0, evs[2]["ocupacion"])
		actual, err := db.Clase(ctx, c.ID, "")
		require.NoError(t, err)
		require.EqualValues(t, 3, actual.Version)
	})

	t.Run("reserva_rechazada_no_deja_evento", func(t *testing.T) {
		antes := len(eventos(c.ID))
		_, _, err := db.Reservar(ctx, op(uuidN(902), "busqueda-reserva-02"), c.ID, func(domain.Clase) error { return domain.ErrSinCupo })
		require.ErrorIs(t, err, domain.ErrSinCupo)
		require.Len(t, eventos(c.ID), antes, "rollback: ni versión ni evento")
	})

	t.Run("renombrar_actividad_republica_sus_clases", func(t *testing.T) {
		antes := len(eventos(c.ID))
		_, err := db.ModificarActividad(ctx, uuidN(1), zumba.ID, func(a domain.Actividad, _ bool) (domain.Actividad, error) {
			a.Nombre = "Zumba Fitness"
			return a, nil
		})
		require.NoError(t, err)
		evs := eventos(c.ID)
		require.Len(t, evs, antes+1)
		require.Equal(t, "Zumba Fitness", evs[len(evs)-1]["actividad_nombre"])
		_, err = db.ModificarActividad(ctx, uuidN(1), zumba.ID, func(a domain.Actividad, _ bool) (domain.Actividad, error) {
			a.CapacidadMaxima = 25
			return a, nil
		})
		require.NoError(t, err)
		require.Len(t, eventos(c.ID), antes+1, "un cambio que no altera el documento no republica")
	})

	t.Run("asistencia_sigue_unica_por_reserva", func(t *testing.T) {
		_, err := db.pool.Exec(ctx, `INSERT INTO outbox(event_type,occurred_at,agregado_id,data) VALUES('asistencia.registrada',now(),$1,'{}'),('asistencia.registrada',now(),$1,'{}')`, uuidN(903))
		require.Error(t, err, "el índice parcial conserva un evento por reserva")
	})

	t.Run("marcas_fuente_y_resumen_para_reindexar", func(t *testing.T) {
		otra := clase(t, db, zumba, uuidN(904), inicio.Add(3*time.Hour), 60, 5)
		_, _, err := reservar(db, uuidN(905), "busqueda-reserva-03", otra.ID)
		require.NoError(t, err)
		marcas, err := db.ReservadasPor(ctx, uuidN(905), []string{c.ID, otra.ID})
		require.NoError(t, err)
		require.Equal(t, map[string]bool{otra.ID: true}, marcas)

		cantidad, suma, err := db.ResumenClases(ctx)
		require.NoError(t, err)
		var leidas, sumaLeida int64
		lotes := 0
		require.NoError(t, db.RecorrerClases(ctx, 1, func(lote []domain.Clase) error {
			lotes++
			for _, cl := range lote {
				leidas++
				sumaLeida += cl.Version
				require.NotEmpty(t, cl.ActividadNombre)
			}
			return nil
		}))
		require.Equal(t, cantidad, leidas)
		require.Equal(t, suma, sumaLeida)
		require.EqualValues(t, cantidad, lotes, "lotes de 1 recorren todas las clases")
	})
}
