package postgres

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/system"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/application"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/stretchr/testify/require"
)

// Cuponera interna (D-94) con PostgreSQL real: HU-09 (CA-09.3/4/5), HU-20
// (CA-20.1 a 20.4), RN-23/24/25, CL-07, CL-07b y las invariantes de la base.
func verificarCuponera(t *testing.T, db *DB) {
	ctx := context.Background()
	ids := system.IDs{}
	uuid := func() string { id, err := ids.NewID(); require.NoError(t, err); return id }
	cuponera := application.NewCuponera(db, system.Clock{}, ids, system.Codigos{})
	admin := application.Actor{ID: uuid(), Rol: domain.RolAdministrador}
	hoy := domain.DiaNegocio(time.Now())

	// alumnoCon crea una cuenta con saldo acreditado por un partner.
	alumnoCon := func(t *testing.T, puntos int64) (application.Actor, fixture) {
		f := nuevaFixture(t, db)
		_, err := f.app.Acreditar(ctx, f.credito("credito-"+uuid()[:8], puntos))
		require.NoError(t, err)
		var user string
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT alumno_id::text FROM cuentas WHERE id=$1`, f.cuenta).Scan(&user))
		return application.Actor{ID: user, Rol: domain.RolAlumno}, f
	}
	saldo := func(f fixture) int64 {
		var s int64
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT saldo FROM cuentas WHERE id=$1`, f.cuenta).Scan(&s))
		return s
	}
	contar := func(query string, args ...any) int {
		var n int
		require.NoError(t, db.pool.QueryRow(ctx, query, args...).Scan(&n))
		return n
	}
	nuevoBeneficio := func(t *testing.T, nombre string, costo int64) domain.Beneficio {
		b, err := cuponera.CrearBeneficio(ctx, admin, domain.Beneficio{Nombre: nombre, Descripcion: "Para la prueba", Costo: costo, VigenteDesde: hoy, Activo: true})
		require.NoError(t, err)
		return b
	}

	t.Run("CA-20.1_y_20.4_canje_idempotente", func(t *testing.T) {
		b := nuevoBeneficio(t, "Botella", 30)
		alumno, f := alumnoCon(t, 50)
		out, repetido, err := cuponera.Canjear(ctx, alumno, b.ID, "canje-clave-0001")
		require.NoError(t, err)
		require.False(t, repetido)
		require.EqualValues(t, 20, out.Saldo)
		require.Equal(t, domain.CanjeConfirmado, out.Canje.Estado)
		require.True(t, domain.CodigoCanjeValido(out.Canje.Codigo))
		require.EqualValues(t, 20, saldo(f))
		require.Equal(t, 1, contar(`SELECT count(*) FROM movimientos_puntos WHERE cuenta_id=$1 AND tipo='CANJE' AND puntos=-30 AND origen_id=$2`, f.cuenta, out.Canje.ID))

		// Doble clic: misma clave, un solo canje y un solo débito.
		again, repetido, err := cuponera.Canjear(ctx, alumno, b.ID, "canje-clave-0001")
		require.NoError(t, err)
		require.True(t, repetido)
		require.Equal(t, out.Canje.ID, again.Canje.ID)
		require.EqualValues(t, 20, again.Saldo)
		require.Equal(t, 1, contar(`SELECT count(*) FROM canjes WHERE usuario_id=$1`, alumno.ID))

		otro := nuevoBeneficio(t, "Toalla", 10)
		_, _, err = cuponera.Canjear(ctx, alumno, otro.ID, "canje-clave-0001")
		require.ErrorIs(t, err, domain.ErrConflictoIdempotencia)

		// CA-20.2 / CL-07: saldo 20, beneficio 30 → rechazo sin cambios.
		_, _, err = cuponera.Canjear(ctx, alumno, b.ID, "canje-clave-0002")
		require.ErrorIs(t, err, domain.ErrSaldoInsuficiente)
		require.EqualValues(t, 20, saldo(f))
		require.Equal(t, 1, contar(`SELECT count(*) FROM canjes WHERE usuario_id=$1`, alumno.ID))

		mios, err := cuponera.MisCanjes(ctx, alumno, 1, 20)
		require.NoError(t, err)
		require.EqualValues(t, 1, mios.Total)
		porCodigo, err := cuponera.ListarCanjes(ctx, admin, out.Canje.Codigo, "", 1, 20)
		require.NoError(t, err)
		require.Len(t, porCodigo.Items, 1)
		require.Equal(t, alumno.ID, porCodigo.Items[0].UsuarioID)
	})

	t.Run("CL-07b_canjes_concurrentes_no_dejan_saldo_negativo", func(t *testing.T) {
		b := nuevoBeneficio(t, "Remera", 30)
		alumno, f := alumnoCon(t, 50)
		var wg sync.WaitGroup
		errs := make(chan error, 4)
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, _, err := cuponera.Canjear(ctx, alumno, b.ID, "concurrente-000"+string(rune('a'+i)))
				errs <- err
			}(i)
		}
		wg.Wait()
		close(errs)
		ok := 0
		for err := range errs {
			if err == nil {
				ok++
			} else {
				require.ErrorIs(t, err, domain.ErrSaldoInsuficiente)
			}
		}
		require.Equal(t, 1, ok)
		require.EqualValues(t, 20, saldo(f))
	})

	t.Run("sin_cuenta_de_puntos_es_saldo_insuficiente", func(t *testing.T) {
		b := nuevoBeneficio(t, "Llavero", 5)
		_, _, err := cuponera.Canjear(ctx, application.Actor{ID: uuid(), Rol: domain.RolAlumno}, b.ID, "sin-cuenta-0001")
		require.ErrorIs(t, err, domain.ErrSaldoInsuficiente)
		_, _, err = cuponera.Canjear(ctx, admin, b.ID, "admin-canje-0001")
		require.ErrorIs(t, err, domain.ErrSinPermiso, "solo el alumno canjea")
	})

	t.Run("CA-09.3_4_5_catalogo_sin_stock_y_cambios", func(t *testing.T) {
		b := nuevoBeneficio(t, "Batido proteico", 30)
		alumno, f := alumnoCon(t, 200)
		primero, _, err := cuponera.Canjear(ctx, alumno, b.ID, "catalogo-0001")
		require.NoError(t, err)
		_, _, err = cuponera.Canjear(ctx, alumno, b.ID, "catalogo-0002")
		require.NoError(t, err, "sin stock: se canjea otra vez (CA-09.5)")

		b.Costo = 40
		_, err = cuponera.ActualizarBeneficio(ctx, admin, b.ID, b)
		require.NoError(t, err)
		nuevo, _, err := cuponera.Canjear(ctx, alumno, b.ID, "catalogo-0003")
		require.NoError(t, err)
		require.EqualValues(t, 40, nuevo.Canje.Puntos)
		require.EqualValues(t, 200-30-30-40, saldo(f))
		var anterior int64
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT puntos FROM canjes WHERE id=$1`, primero.Canje.ID).Scan(&anterior))
		require.EqualValues(t, 30, anterior, "el canje previo conserva lo debitado (CA-09.4)")

		b.Activo = false
		_, err = cuponera.ActualizarBeneficio(ctx, admin, b.ID, b)
		require.NoError(t, err)
		catalogo, err := cuponera.Catalogo(ctx, alumno)
		require.NoError(t, err)
		for _, item := range catalogo {
			require.NotEqual(t, b.ID, item.ID, "un inactivo no aparece en el catálogo (CA-09.3)")
		}
		_, _, err = cuponera.Canjear(ctx, alumno, b.ID, "catalogo-0004")
		require.ErrorIs(t, err, domain.ErrBeneficioNoDisponible, "CA-20.3")
		require.Equal(t, 3, contar(`SELECT count(*) FROM canjes WHERE beneficio_id=$1`, b.ID), "los canjes previos se conservan")
		todos, err := cuponera.ListarBeneficios(ctx, admin)
		require.NoError(t, err)
		require.NotEmpty(t, todos)
	})

	t.Run("RN-25_usar_y_anular_cupones", func(t *testing.T) {
		b := nuevoBeneficio(t, "Clase de prueba", 30)
		alumno, f := alumnoCon(t, 100)
		usado, _, err := cuponera.Canjear(ctx, alumno, b.ID, "cupon-uso-0001")
		require.NoError(t, err)
		anulable, _, err := cuponera.Canjear(ctx, alumno, b.ID, "cupon-anula-0001")
		require.NoError(t, err)
		require.EqualValues(t, 40, saldo(f))

		k, cambio, err := cuponera.UsarCanje(ctx, admin, usado.Canje.ID)
		require.NoError(t, err)
		require.True(t, cambio)
		require.Equal(t, domain.CanjeUsado, k.Estado)
		_, cambio, err = cuponera.UsarCanje(ctx, admin, usado.Canje.ID)
		require.NoError(t, err)
		require.False(t, cambio, "repetir el uso no tiene efectos")
		_, _, err = cuponera.AnularCanje(ctx, admin, usado.Canje.ID, "Ya entregado")
		require.ErrorIs(t, err, domain.ErrTransicionCanje, "un cupón usado no se anula")

		out, cambio, err := cuponera.AnularCanje(ctx, admin, anulable.Canje.ID, "Se equivocó de beneficio")
		require.NoError(t, err)
		require.True(t, cambio)
		require.Equal(t, domain.CanjeAnulado, out.Canje.Estado)
		require.EqualValues(t, 70, out.Saldo)
		require.EqualValues(t, 70, saldo(f))
		require.Equal(t, 1, contar(`SELECT count(*) FROM movimientos_puntos WHERE revierte_id=$1 AND tipo='REVERSION' AND puntos=30`, anulable.Canje.MovimientoID))
		_, cambio, err = cuponera.AnularCanje(ctx, admin, anulable.Canje.ID, "Se equivocó de beneficio")
		require.NoError(t, err)
		require.False(t, cambio, "anular dos veces devuelve el estado sin otro compensatorio")
		require.EqualValues(t, 70, saldo(f))
		_, _, err = cuponera.UsarCanje(ctx, admin, anulable.Canje.ID)
		require.ErrorIs(t, err, domain.ErrTransicionCanje)
		require.Equal(t, 2, contar(`SELECT count(*) FROM auditoria_club WHERE accion IN ('USO_CANJE','ANULACION_CANJE') AND detalle->>'canje_id' IN ($1,$2)`, usado.Canje.ID, anulable.Canje.ID))

		// La reversión genérica del administrador no toca el débito de un canje.
		_, _, err = application.NewAdministracion(db, system.Clock{}, ids, system.Secrets{}).Revertir(ctx, admin, usado.Canje.MovimientoID, "Corrección manual", "revertir-canje-0001")
		require.ErrorIs(t, err, domain.ErrCanjeSoloPorAnulacion)
	})

	t.Run("invariantes_garantizadas_por_la_base", func(t *testing.T) {
		b := nuevoBeneficio(t, "Gorra", 10)
		alumno, f := alumnoCon(t, 50)
		usado, _, err := cuponera.Canjear(ctx, alumno, b.ID, "invariante-0001")
		require.NoError(t, err)
		_, _, err = cuponera.UsarCanje(ctx, admin, usado.Canje.ID)
		require.NoError(t, err)

		_, err = db.pool.Exec(ctx, `UPDATE canjes SET estado='CONFIRMADO', usado_en=NULL WHERE id=$1`, usado.Canje.ID)
		require.Error(t, err, "un cupón usado no vuelve a confirmado")
		_, err = db.pool.Exec(ctx, `UPDATE canjes SET puntos=1 WHERE id=$1`, usado.Canje.ID)
		require.Error(t, err, "lo debitado es inmutable")
		_, err = db.pool.Exec(ctx, `DELETE FROM canjes WHERE id=$1`, usado.Canje.ID)
		require.Error(t, err, "los canjes no se borran")

		// Un débito CANJE sin su cupón no se confirma.
		tx, err := db.pool.Begin(ctx)
		require.NoError(t, err)
		_, err = tx.Exec(ctx, `UPDATE cuentas SET saldo=saldo-5, ultima_secuencia=ultima_secuencia+1 WHERE id=$1`, f.cuenta)
		require.NoError(t, err)
		_, err = tx.Exec(ctx, `INSERT INTO movimientos_puntos(id,cuenta_id,secuencia,puntos,motivo,referencia,ocurrido_en,aplicado_en,tipo,origen_id)
			SELECT $1,$2,ultima_secuencia,-5,'Canje: falso','ABCD-EF23',now(),now(),'CANJE',$3 FROM cuentas WHERE id=$2`, uuid(), f.cuenta, uuid())
		require.NoError(t, err)
		require.Error(t, tx.Commit(ctx), "débito de canje sin cupón")

		// Una reversión directa del débito de un canje no anulado tampoco.
		tx, err = db.pool.Begin(ctx)
		require.NoError(t, err)
		_, err = tx.Exec(ctx, `UPDATE cuentas SET saldo=saldo+10, ultima_secuencia=ultima_secuencia+1 WHERE id=$1`, f.cuenta)
		require.NoError(t, err)
		_, err = tx.Exec(ctx, `INSERT INTO movimientos_puntos(id,cuenta_id,secuencia,puntos,motivo,referencia,ocurrido_en,aplicado_en,tipo,revierte_id)
			SELECT $1,$2,ultima_secuencia,10,'Atajo','x',now(),now(),'REVERSION',$3 FROM cuentas WHERE id=$2`, uuid(), f.cuenta, usado.Canje.MovimientoID)
		require.NoError(t, err)
		require.Error(t, tx.Commit(ctx), "un canje solo se revierte anulándolo")
		require.EqualValues(t, 40, saldo(f))
	})
}
