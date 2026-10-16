package postgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/system"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/application"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/ports"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

type relojPrueba struct{ ahora time.Time }

func (r relojPrueba) Now() time.Time { return r.ahora }

type falloAlCompletar struct{ ports.Transaction }

func (falloAlCompletar) CompletarOperacion(context.Context, string, string, domain.ResultadoOperacion) error {
	return errors.New("fallo de prueba antes de confirmar idempotencia")
}

type ledgerConFallo struct{ *DB }

func (l ledgerConFallo) EnTransaccion(ctx context.Context, fn func(ports.Transaction) error) error {
	return l.DB.EnTransaccion(ctx, func(tx ports.Transaction) error { return fn(falloAlCompletar{tx}) })
}

type fixture struct {
	db                             *DB
	app                            *application.Benefits
	partner, otro, cuenta, usuario string
}

func nuevaFixture(t *testing.T, db *DB) fixture {
	t.Helper()
	ids := system.IDs{}
	nuevo := func() string { id, err := ids.NewID(); require.NoError(t, err); return id }
	f := fixture{db: db, partner: nuevo(), otro: nuevo(), cuenta: nuevo(), usuario: "alumno-1"}
	tx, err := db.pool.Begin(context.Background())
	require.NoError(t, err)
	defer tx.Rollback(context.Background())
	for _, id := range []string{f.partner, f.otro} {
		hash := sha256.Sum256([]byte("fixture-" + id))
		_, err = tx.Exec(context.Background(), `INSERT INTO partners(id,nombre,api_key_hash) VALUES ($1,'Partner de prueba',$2)`, id, hash[:])
		require.NoError(t, err)
	}
	_, err = tx.Exec(context.Background(), `INSERT INTO cuentas(id,alumno_id) VALUES ($1,$2)`, f.cuenta, nuevo())
	require.NoError(t, err)
	for _, id := range []string{f.partner, f.otro} {
		_, err = tx.Exec(context.Background(), `INSERT INTO vinculaciones(partner_id,external_user_id,cuenta_id) VALUES ($1,$2,$3)`, id, f.usuario, f.cuenta)
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit(context.Background()))
	f.app = application.New(db, relojPrueba{time.Now().UTC().Truncate(time.Microsecond)}, ids)
	return f
}

func (f fixture) credito(clave string, puntos int64) application.AcreditarPuntos {
	return application.AcreditarPuntos{PartnerID: f.partner, ExternalUserID: f.usuario, Clave: clave, Puntos: puntos, Motivo: "Compra de prueba", Referencia: "ref-1"}
}

func TestLedger_PostgreSQLReal(t *testing.T) {
	requireDocker(t)
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine", tcpostgres.WithDatabase("benefits_db"), tcpostgres.WithUsername("benefits_user"), tcpostgres.WithPassword("test-password"), testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(90*time.Second)))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ctr.Terminate(context.Background())) })
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	admin, err := Open(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(admin.Close)
	// El usuario creado por la imagen es superusuario. La prueba funcional usa
	// un rol sin privilegios globales, igual que el aislamiento del despliegue.
	_, err = admin.pool.Exec(ctx, `CREATE ROLE ledger_user LOGIN PASSWORD 'fixture-ledger' NOSUPERUSER NOCREATEDB NOCREATEROLE`)
	require.NoError(t, err)
	_, err = admin.pool.Exec(ctx, `CREATE DATABASE isolated_benefits_db OWNER ledger_user`)
	require.NoError(t, err)
	_, err = admin.pool.Exec(ctx, `REVOKE CONNECT ON DATABASE benefits_db FROM PUBLIC`)
	require.NoError(t, err)
	conexion, err := url.Parse(dsn)
	require.NoError(t, err)
	conexion.User = url.UserPassword("ledger_user", "fixture-ledger")
	conexion.Path = "/isolated_benefits_db"
	dsn = conexion.String()
	db, err := Open(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(db.Close)
	require.NoError(t, db.Migrate(ctx))
	require.NoError(t, db.Migrate(ctx)) // Desde base vacía y repetición segura.
	var migraciones int
	require.NoError(t, db.pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&migraciones))
	require.Equal(t, 1, migraciones)
	t.Run("D3_usuario_no_accede_a_base_ajena", func(t *testing.T) {
		otra := *conexion
		otra.Path = "/benefits_db"
		ajena, err := Open(ctx, otra.String())
		require.NoError(t, err)
		defer ajena.Close()
		require.Error(t, ajena.Ping(ctx))
	})

	t.Run("RN22_RN27_replay_durable_con_saldo_original", func(t *testing.T) {
		f := nuevaFixture(t, db)
		primera, err := f.app.Acreditar(ctx, f.credito("compra-0001", 100))
		require.NoError(t, err)
		_, err = f.app.Acreditar(ctx, f.credito("compra-0002", 50))
		require.NoError(t, err)
		// Un pool nuevo simula otro proceso; no existe caché de respuestas.
		otraDB, err := Open(ctx, dsn)
		require.NoError(t, err)
		defer otraDB.Close()
		otroProceso := application.New(otraDB, system.Clock{}, system.IDs{})
		replay, err := otroProceso.Acreditar(ctx, f.credito("compra-0001", 100))
		require.NoError(t, err)
		require.True(t, replay.Repetida)
		require.Equal(t, primera.Resultado, replay.Resultado)
		require.EqualValues(t, 100, replay.Resultado.Saldo)
		saldo, err := otroProceso.ConsultarSaldo(ctx, f.partner, f.usuario)
		require.NoError(t, err)
		require.EqualValues(t, 150, saldo)
		_, err = f.app.Acreditar(ctx, f.credito("compra-0001", 101))
		require.ErrorIs(t, err, domain.ErrConflictoIdempotencia)
		otroDestino := f.credito("compra-0001", 100)
		otroDestino.ExternalUserID = "otra-cuenta"
		_, err = f.app.Acreditar(ctx, otroDestino)
		require.ErrorIs(t, err, domain.ErrConflictoIdempotencia)
		pagina, err := f.app.ListarMovimientos(ctx, f.partner, f.usuario, 1, 1)
		require.NoError(t, err)
		require.EqualValues(t, 2, pagina.Total)
		require.Len(t, pagina.Movimientos, 1)
		require.EqualValues(t, 50, pagina.Movimientos[0].Datos().Puntos)
		pagina, err = f.app.ListarMovimientos(ctx, f.partner, f.usuario, 3, 1)
		require.NoError(t, err)
		require.Empty(t, pagina.Movimientos)
	})
	t.Run("RN26_CL08c_saldo_total_historial_aislado", func(t *testing.T) {
		f := nuevaFixture(t, db)
		_, err := f.app.Acreditar(ctx, f.credito("clave-compartida", 100))
		require.NoError(t, err)
		otra := f.credito("clave-compartida", 200)
		otra.PartnerID = f.otro
		_, err = f.app.Acreditar(ctx, otra)
		require.NoError(t, err)
		for _, partner := range []string{f.partner, f.otro} {
			saldo, err := f.app.ConsultarSaldo(ctx, partner, f.usuario)
			require.NoError(t, err)
			require.EqualValues(t, 300, saldo)
			p, err := f.app.ListarMovimientos(ctx, partner, f.usuario, 1, 20)
			require.NoError(t, err)
			require.EqualValues(t, 1, p.Total)
			require.Equal(t, partner, p.Movimientos[0].Datos().PartnerID)
		}
		_, err = f.app.ConsultarSaldo(ctx, f.partner, "no-vinculado")
		require.ErrorIs(t, err, domain.ErrCuentaNoVinculada)
		_, err = f.app.ListarMovimientos(ctx, f.partner, "no-vinculado", 1, 20)
		require.ErrorIs(t, err, domain.ErrCuentaNoVinculada)
	})
	t.Run("RN27_CL08b_rechazo_original_tras_vincular", func(t *testing.T) {
		f := nuevaFixture(t, db)
		c := f.credito("rechazo-0001", 100)
		c.ExternalUserID = "futuro"
		primera, err := f.app.Acreditar(ctx, c)
		require.ErrorIs(t, err, domain.ErrCuentaNoVinculada)
		// Otra cuenta permite vincular el ID sin modificar la vinculación existente.
		id, _ := (system.IDs{}).NewID()
		alumno, _ := (system.IDs{}).NewID()
		_, err = db.pool.Exec(ctx, `INSERT INTO cuentas(id,alumno_id) VALUES ($1,$2)`, id, alumno)
		require.NoError(t, err)
		_, err = db.pool.Exec(ctx, `INSERT INTO vinculaciones(partner_id,external_user_id,cuenta_id) VALUES ($1,'futuro',$2)`, f.partner, id)
		require.NoError(t, err)
		replay, err := f.app.Acreditar(ctx, c)
		require.ErrorIs(t, err, domain.ErrCuentaNoVinculada)
		require.True(t, replay.Repetida)
		require.Equal(t, primera.Resultado, replay.Resultado)
		c.Clave = "rechazo-0002"
		_, err = f.app.Acreditar(ctx, c)
		require.NoError(t, err)
		invalida := f.credito("invalida-001", 0)
		_, err = f.app.Acreditar(ctx, invalida)
		require.ErrorIs(t, err, domain.ErrOperacionInvalida)
		r, err := f.app.Acreditar(ctx, invalida)
		require.ErrorIs(t, err, domain.ErrOperacionInvalida)
		require.True(t, r.Repetida)
	})
	t.Run("RN27_concurrencia_misma_clave_y_claves_distintas", func(t *testing.T) {
		f := nuevaFixture(t, db)
		ejecutar := func(misma bool) {
			var wg sync.WaitGroup
			fallos := make(chan error, 12)
			inicio := make(chan struct{})
			for i := 0; i < 12; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-inicio
					key := "simultanea-unica"
					if !misma {
						key = fmt.Sprintf("simultanea-%04d", i)
					}
					_, err := f.app.Acreditar(ctx, f.credito(key, 10))
					fallos <- err
				}(i)
			}
			close(inicio)
			wg.Wait()
			close(fallos)
			for err := range fallos {
				require.NoError(t, err)
			}
		}
		ejecutar(true)
		p, err := f.app.ListarMovimientos(ctx, f.partner, f.usuario, 1, 100)
		require.NoError(t, err)
		require.EqualValues(t, 1, p.Total)
		ejecutar(false)
		p, err = f.app.ListarMovimientos(ctx, f.partner, f.usuario, 1, 100)
		require.NoError(t, err)
		require.EqualValues(t, 13, p.Total)
		saldo, err := f.app.ConsultarSaldo(ctx, f.partner, f.usuario)
		require.NoError(t, err)
		require.EqualValues(t, 130, saldo)
		var suma int64
		for i, m := range p.Movimientos {
			suma += m.Datos().Puntos
			require.EqualValues(t, 13-i, m.Datos().Secuencia)
		}
		require.Equal(t, suma, saldo)
	})
	t.Run("RN22_RN27_restricciones_y_retencion_en_base", func(t *testing.T) {
		f := nuevaFixture(t, db)
		_, err := f.app.Acreditar(ctx, f.credito("restriccion-001", 100))
		require.NoError(t, err)
		for _, sql := range []string{
			`UPDATE cuentas SET saldo=-1 WHERE id=$1`,
			`UPDATE cuentas SET saldo=999 WHERE id=$1`,
			`UPDATE movimientos_puntos SET puntos=200 WHERE cuenta_id=$1`,
			`DELETE FROM movimientos_puntos WHERE cuenta_id=$1`,
		} {
			_, err = db.pool.Exec(ctx, sql, f.cuenta)
			var pg *pgconn.PgError
			require.True(t, errors.As(err, &pg))
			require.Equal(t, "23514", pg.Code)
		}
		_, err = db.pool.Exec(ctx, `DELETE FROM operaciones_partner WHERE partner_id=$1`, f.partner)
		require.Error(t, err)
		_, err = db.pool.Exec(ctx, `UPDATE operaciones_partner SET resultado='{}' WHERE partner_id=$1`, f.partner)
		require.Error(t, err)
		var retenida bool
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT conservar_hasta >= creada_en+interval '30 days' FROM operaciones_partner WHERE partner_id=$1`, f.partner).Scan(&retenida))
		require.True(t, retenida)
		// Una transacción fallida no deja ni movimiento ni idempotencia incompleta.
		tx, err := db.pool.Begin(ctx)
		require.NoError(t, err)
		_, err = tx.Exec(ctx, `INSERT INTO operaciones_partner(partner_id,clave,huella,creada_en,conservar_hasta) VALUES ($1,'incompleta-01',repeat('a',64),now(),now()+interval '30 days')`, f.partner)
		require.NoError(t, err)
		require.Error(t, tx.Commit(ctx))
		var count int
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT count(*) FROM operaciones_partner WHERE partner_id=$1 AND clave='incompleta-01'`, f.partner).Scan(&count))
		require.Zero(t, count)
		_, err = db.pool.Exec(ctx, `UPDATE partners SET activo=false WHERE id=$1`, f.partner)
		require.NoError(t, err)
		_, err = f.app.Acreditar(ctx, f.credito("restriccion-001", 100))
		require.ErrorIs(t, err, domain.ErrPartnerNoAutorizado)
	})
	t.Run("RN22_RN27_rollback_del_movimiento_y_la_clave", func(t *testing.T) {
		f := nuevaFixture(t, db)
		appConFallo := application.New(ledgerConFallo{db}, system.Clock{}, system.IDs{})
		_, err := appConFallo.Acreditar(ctx, f.credito("rollback-001", 100))
		require.Error(t, err)
		saldo, err := f.app.ConsultarSaldo(ctx, f.partner, f.usuario)
		require.NoError(t, err)
		require.Zero(t, saldo)
		p, err := f.app.ListarMovimientos(ctx, f.partner, f.usuario, 1, 20)
		require.NoError(t, err)
		require.Zero(t, p.Total)
		var count int
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT count(*) FROM operaciones_partner WHERE partner_id=$1`, f.partner).Scan(&count))
		require.Zero(t, count)
		_, err = f.app.Acreditar(ctx, f.credito("rollback-001", 100))
		require.NoError(t, err)
	})
	t.Run("RN27_payloads_distintos_con_la_misma_clave_concurrente", func(t *testing.T) {
		f := nuevaFixture(t, db)
		inicio := make(chan struct{})
		resultados := make(chan error, 2)
		for _, puntos := range []int64{100, 200} {
			go func(puntos int64) {
				<-inicio
				_, err := f.app.Acreditar(ctx, f.credito("conflicto-001", puntos))
				resultados <- err
			}(puntos)
		}
		close(inicio)
		a, b := <-resultados, <-resultados
		if a == nil {
			require.ErrorIs(t, b, domain.ErrConflictoIdempotencia)
		} else {
			require.ErrorIs(t, a, domain.ErrConflictoIdempotencia)
			require.NoError(t, b)
		}
		p, err := f.app.ListarMovimientos(ctx, f.partner, f.usuario, 1, 20)
		require.NoError(t, err)
		require.EqualValues(t, 1, p.Total)
		saldo, err := f.app.ConsultarSaldo(ctx, f.partner, f.usuario)
		require.NoError(t, err)
		require.Equal(t, p.Movimientos[0].Datos().Puntos, saldo)
	})
}
