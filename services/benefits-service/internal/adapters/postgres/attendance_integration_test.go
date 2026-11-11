package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/system"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/application"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func verificarBooking(t *testing.T, db *DB) {
	ctx := context.Background()
	ids := system.IDs{}
	uuid := func() string { id, err := ids.NewID(); require.NoError(t, err); return id }
	app := application.NewBookingEvents(db, system.Clock{}, ids)
	hecho := func(user, kind string) domain.HechoBooking {
		reservation := uuid()
		return domain.HechoBooking{EventoID: uuid(), Tipo: kind, AlumnoID: user, OrigenID: reservation, ReservaID: reservation, ClaseID: uuid(), OcurridoEn: time.Now().UTC().Truncate(time.Microsecond)}
	}
	check := func(user string, balance, seq int64) {
		var saldo, count, maxSeq, sum int64
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT c.saldo,c.ultima_secuencia,count(m.id),COALESCE(sum(m.puntos),0) FROM cuentas c LEFT JOIN movimientos_puntos m ON m.cuenta_id=c.id WHERE c.alumno_id=$1 GROUP BY c.id`, user).Scan(&saldo, &maxSeq, &count, &sum))
		require.Equal(t, balance, saldo)
		require.Equal(t, saldo, sum)
		require.Equal(t, seq, count)
		require.Equal(t, seq, maxSeq)
	}
	for _, initial := range []int64{0, 50, 500} {
		t.Run(fmt.Sprintf("penalizacion_desde_%d_y_replay", initial), func(t *testing.T) {
			f := nuevaFixture(t, db)
			var user string
			require.NoError(t, db.pool.QueryRow(ctx, `SELECT alumno_id::text FROM cuentas WHERE id=$1`, f.cuenta).Scan(&user))
			seq := int64(0)
			if initial > 0 {
				_, err := f.app.Acreditar(ctx, f.credito("booking-seed-001", initial))
				require.NoError(t, err)
				seq++
			}
			h := hecho(user, domain.PenalizacionInasistencia)
			require.NoError(t, app.Aplicar(ctx, h))
			seq++
			check(user, max(int64(0), initial-100), seq)
			require.NoError(t, app.Aplicar(ctx, h))
			h.EventoID = uuid()
			h.OcurridoEn = h.OcurridoEn.In(time.FixedZone("fixture", -3*3600))
			require.NoError(t, app.Aplicar(ctx, h))
			check(user, max(int64(0), initial-100), seq)
			credit := hecho(user, domain.Asistencia)
			require.NoError(t, app.Aplicar(ctx, credit))
			seq++
			require.NoError(t, app.Aplicar(ctx, h))
			check(user, max(int64(0), initial-100)+500, seq)
			h.AlumnoID = uuid()
			require.ErrorIs(t, app.Aplicar(ctx, h), domain.ErrConflictoIdempotencia)
			_, err := db.pool.Exec(ctx, `DELETE FROM mensajes_booking_procesados WHERE event_id=$1`, credit.EventoID)
			require.Error(t, err)
			page, err := app.Historial(ctx, user, 1, 20)
			require.NoError(t, err)
			require.Equal(t, seq, page.Total)
			require.Equal(t, seq, page.Movimientos[0].Datos().Secuencia)
			partnerPage, err := f.app.ListarMovimientos(ctx, f.partner, f.usuario, 1, 20)
			require.NoError(t, err)
			if initial > 0 {
				require.EqualValues(t, 1, partnerPage.Total)
			} else {
				require.Zero(t, partnerPage.Total)
			}
		})
	}
	t.Run("diez_reentregas_concurrentes_un_efecto", func(t *testing.T) {
		user := uuid()
		h := hecho(user, domain.Asistencia)
		var wg sync.WaitGroup
		errs := make(chan error, 10)
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); errs <- app.Aplicar(ctx, h) }()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			require.NoError(t, err)
		}
		check(user, 500, 1)
	})
	t.Run("credito_HTTP_y_penalizacion_serializan_misma_cuenta", func(t *testing.T) {
		f := nuevaFixture(t, db)
		var user string
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT alumno_id::text FROM cuentas WHERE id=$1`, f.cuenta).Scan(&user))
		h := hecho(user, domain.PenalizacionInasistencia)
		errs := make(chan error, 2)
		go func() { _, err := f.app.Acreditar(ctx, f.credito("booking-race-001", 500)); errs <- err }()
		go func() { errs <- app.Aplicar(ctx, h) }()
		require.NoError(t, <-errs)
		require.NoError(t, <-errs)
		var balance int64
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT saldo FROM cuentas WHERE alumno_id=$1`, user).Scan(&balance))
		require.Contains(t, []int64{400, 500}, balance)
		check(user, balance, 2)
	})
	t.Run("fallo_antes_commit_revierte_cuenta_y_recibo", func(t *testing.T) {
		h := hecho(uuid(), domain.Asistencia)
		failure := errors.New("fixture antes del commit")
		err := db.AplicarHecho(ctx, h, "fixture", func(c domain.CuentaBeneficios) (domain.CuentaBeneficios, domain.MovimientoPuntos, error) {
			return c, domain.MovimientoPuntos{}, failure
		}, nil)
		require.ErrorIs(t, err, failure)
		var count int
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT count(*) FROM cuentas WHERE alumno_id=$1`, h.AlumnoID).Scan(&count))
		require.Zero(t, count)
		require.NoError(t, app.Aplicar(ctx, h))
		check(h.AlumnoID, 500, 1)
	})
	t.Run("base_rechaza_debito_partner_y_movimiento_sin_recibo", func(t *testing.T) {
		f := nuevaFixture(t, db)
		_, err := db.pool.Exec(ctx, `INSERT INTO movimientos_puntos(id,cuenta_id,partner_id,secuencia,puntos,motivo,referencia,ocurrido_en,aplicado_en) VALUES(gen_random_uuid(),$1,$2,1,-1,'Invalido','fixture',now(),now())`, f.cuenta, f.partner)
		require.Error(t, err)
		tx, err := db.pool.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback(ctx) }()
		_, err = tx.Exec(ctx, `INSERT INTO movimientos_puntos(id,cuenta_id,secuencia,puntos,motivo,referencia,ocurrido_en,aplicado_en,tipo,origen_id) VALUES(gen_random_uuid(),$1,1,500,'Sin recibo','fixture',now(),now(),'ASISTENCIA',gen_random_uuid())`, f.cuenta)
		require.NoError(t, err)
		_, err = tx.Exec(ctx, `UPDATE cuentas SET saldo=500,ultima_secuencia=1 WHERE id=$1`, f.cuenta)
		require.NoError(t, err)
		require.Error(t, tx.Commit(ctx))
	})
}
