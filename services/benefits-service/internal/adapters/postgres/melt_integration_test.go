package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/system"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/application"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
)

// Etapa 13 (D-92) contra PostgreSQL real: el envío a MELT nace en la misma
// transacción que la asistencia, una reentrega no lo duplica, las ausencias no
// se envían y la operación enviada es inmutable.
func verificarMelt(t *testing.T, db *DB) {
	ctx := context.Background()
	ids := system.IDs{}
	uuid := func() string { id, err := ids.NewID(); require.NoError(t, err); return id }
	app := application.NewBookingEvents(db, system.Clock{}, ids).ConEnviosMelt()
	alumno := uuid()
	hecho := func(kind string) domain.HechoBooking {
		r := uuid()
		return domain.HechoBooking{EventoID: uuid(), Tipo: kind, AlumnoID: alumno, OrigenID: r, ReservaID: r, ClaseID: uuid(), OcurridoEn: time.Now().UTC().Truncate(time.Microsecond)}
	}
	count := func() int {
		var n int
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT count(*) FROM envios_melt WHERE alumno_id=$1`, alumno).Scan(&n))
		return n
	}

	asistencia := hecho(domain.Asistencia)
	require.NoError(t, app.Aplicar(ctx, asistencia))
	require.Equal(t, 1, count(), "la asistencia registra su envío")
	require.NoError(t, app.Aplicar(ctx, asistencia), "reentrega del mismo evento")
	otra := asistencia
	otra.EventoID = uuid()
	require.NoError(t, app.Aplicar(ctx, otra), "mismo hecho con otro event_id")
	require.Equal(t, 1, count(), "una reentrega no duplica el envío")

	require.NoError(t, app.Aplicar(ctx, hecho(domain.PenalizacionInasistencia)))
	require.Equal(t, 1, count(), "las ausencias no se envían a MELT")

	envios, err := db.EnviosMeltDeAlumno(ctx, alumno)
	require.NoError(t, err)
	require.Len(t, envios, 1)
	e := envios[0]
	require.Equal(t, int64(100), e.Puntos)
	require.Equal(t, domain.EnvioPendiente, e.Estado)
	require.Equal(t, "metalfitness-asistencia-"+asistencia.OrigenID, e.Referencia)

	vencidos, err := db.EnviosMeltVencidos(ctx, time.Now().Add(time.Second), 50)
	require.NoError(t, err)
	require.Contains(t, referencias(vencidos), e.Referencia)

	ahora := time.Now().UTC().Truncate(time.Microsecond)
	vence := ahora.Add(7 * 24 * time.Hour)
	p := e.Registrar(domain.ResultadoMelt{Tipo: domain.ResultadoPendiente, RegistroURL: "https://melt.example/registro#token", RegistroVence: vence}, ahora)
	require.NoError(t, db.GuardarEnvioMelt(ctx, p))
	vencidos, err = db.EnviosMeltVencidos(ctx, ahora.Add(time.Minute), 50)
	require.NoError(t, err)
	require.NotContains(t, referencias(vencidos), e.Referencia, "un enlace vigente espera la reconsulta")
	vencidos, err = db.EnviosMeltVencidos(ctx, ahora.Add(domain.ReconsultaRegistroMelt), 50)
	require.NoError(t, err)
	require.Contains(t, referencias(vencidos), e.Referencia, "se reconsulta para detectar el registro")
	vencidos, err = db.EnviosMeltVencidos(ctx, vence.Add(time.Minute), 50)
	require.NoError(t, err)
	require.Contains(t, referencias(vencidos), e.Referencia, "un enlace vencido se renueva")

	a := p.Registrar(domain.ResultadoMelt{Tipo: domain.ResultadoAcreditado}, ahora)
	require.NoError(t, db.GuardarEnvioMelt(ctx, a))
	envios, err = db.EnviosMeltDeAlumno(ctx, alumno)
	require.NoError(t, err)
	require.Equal(t, domain.EnvioAcreditado, envios[0].Estado)
	require.Empty(t, envios[0].RegistroURL)
	require.Equal(t, 2, envios[0].Intentos)

	_, err = db.pool.Exec(ctx, `UPDATE envios_melt SET referencia='otra' WHERE id=$1`, e.ID)
	require.Error(t, err, "la referencia enviada a MELT es inmutable")
	_, err = db.pool.Exec(ctx, `UPDATE envios_melt SET puntos=999 WHERE id=$1`, e.ID)
	require.Error(t, err, "los puntos enviados son inmutables")
	_, err = db.pool.Exec(ctx, `DELETE FROM envios_melt WHERE id=$1`, e.ID)
	require.Error(t, err, "los envíos no se borran")
	_, err = db.pool.Exec(ctx, `UPDATE envios_melt SET estado='PENDIENTE_REGISTRO', registro_url=NULL WHERE id=$1`, e.ID)
	require.Error(t, err, "un pendiente de registro siempre tiene enlace")

	// Sin envíos MELT habilitados, la asistencia no registra envío.
	sinMelt := application.NewBookingEvents(db, system.Clock{}, ids)
	h := hecho(domain.Asistencia)
	require.NoError(t, sinMelt.Aplicar(ctx, h))
	var n int
	require.NoError(t, db.pool.QueryRow(ctx, `SELECT count(*) FROM envios_melt WHERE origen_id=$1`, h.OrigenID).Scan(&n))
	require.Zero(t, n)
}

func referencias(envios []domain.EnvioMelt) []string {
	out := make([]string, 0, len(envios))
	for _, e := range envios {
		out = append(out, e.Referencia)
	}
	return out
}
