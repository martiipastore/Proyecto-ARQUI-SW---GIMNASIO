package postgres

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/system"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/application"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/stretchr/testify/require"
)

// Etapa 7 de la entrega final con PostgreSQL real: HU-10 (CA-10.1 a 10.4),
// HU-11 (CA-11.1 a 11.3), CA-35.3, CL-13 y RN-22/23/25/26.
func verificarAdministracion(t *testing.T, db *DB) {
	ctx := context.Background()
	ids := system.IDs{}
	uuid := func() string { id, err := ids.NewID(); require.NoError(t, err); return id }
	actor := application.Actor{ID: uuid(), Rol: domain.RolAdministrador}
	admin := application.NewAdministracion(db, system.Clock{}, ids, system.Secrets{})
	auth := application.NewAuthenticator(db)
	usuarioDe := func(f fixture) string {
		var user string
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT alumno_id::text FROM cuentas WHERE id=$1`, f.cuenta).Scan(&user))
		return user
	}
	auditorias := func(partner string) int {
		var n int
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT count(*) FROM auditoria_club WHERE partner_id=$1`, partner).Scan(&n))
		return n
	}

	t.Run("CA-10.1_y_10.2_alta_rotacion_y_desactivacion", func(t *testing.T) {
		alta, err := admin.CrearIntegracion(ctx, actor, domain.ConfiguracionIntegracion{Nombre: "Estudiario", PermiteConsulta: true, PermitePuntos: true, RequiereEvento: true, MaxPuntosOperacion: 1000, MaxSolicitudesMinuto: 120})
		require.NoError(t, err)
		require.True(t, alta.Integracion.Config.Activo)
		require.Len(t, alta.APIKey, 64)
		id, err := auth.Autenticar(ctx, alta.APIKey)
		require.NoError(t, err)
		require.Equal(t, alta.Integracion.ID, id)
		var hash []byte
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT api_key_hash FROM partners WHERE id=$1`, id).Scan(&hash))
		sum := sha256.Sum256([]byte(alta.APIKey))
		require.Equal(t, sum[:], hash, "solo se guarda el hash")

		rot, err := admin.RotarCredencial(ctx, actor, id)
		require.NoError(t, err)
		_, err = auth.Autenticar(ctx, alta.APIKey)
		require.ErrorIs(t, err, application.ErrCredencialInvalida, "la key anterior queda revocada")
		_, err = auth.Autenticar(ctx, rot.APIKey)
		require.NoError(t, err)

		cfg := alta.Integracion.Config
		cfg.Activo = false
		cambio, err := admin.ActualizarIntegracion(ctx, actor, id, cfg)
		require.NoError(t, err)
		require.False(t, cambio.Config.Activo)
		_, err = auth.Autenticar(ctx, rot.APIKey)
		require.ErrorIs(t, err, application.ErrPartnerInactivo)
		require.Equal(t, 3, auditorias(id))
		var conClave int
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT count(*) FROM auditoria_club WHERE detalle::text LIKE '%'||$1||'%' OR detalle::text LIKE '%'||$2||'%'`, alta.APIKey, rot.APIKey).Scan(&conClave))
		require.Zero(t, conClave, "la auditoría nunca guarda credenciales")
		_, err = db.pool.Exec(ctx, `DELETE FROM auditoria_club WHERE partner_id=$1`, id)
		require.Error(t, err, "auditoría inmutable")

		_, err = admin.ActualizarIntegracion(ctx, actor, uuid(), cfg)
		require.ErrorIs(t, err, domain.ErrIntegracionNoEncontrada)
		lista, err := admin.ListarIntegraciones(ctx, actor)
		require.NoError(t, err)
		require.NotEmpty(t, lista)
	})

	t.Run("CA-10.3_y_10.4_vinculacion", func(t *testing.T) {
		f := nuevaFixture(t, db)
		user := usuarioDe(f)
		alta, err := admin.CrearIntegracion(ctx, actor, domain.ConfiguracionIntegracion{Nombre: "Partner nuevo", PermiteConsulta: true, PermitePuntos: true, MaxPuntosOperacion: 1000, MaxSolicitudesMinuto: 120})
		require.NoError(t, err)
		partner := alta.Integracion.ID
		creada, err := admin.Vincular(ctx, actor, partner, user, "socio-77")
		require.NoError(t, err)
		require.True(t, creada)
		creada, err = admin.Vincular(ctx, actor, partner, user, "socio-77")
		require.NoError(t, err)
		require.False(t, creada, "repetir el mismo vínculo no tiene efectos")
		_, err = admin.Vincular(ctx, actor, partner, user, "socio-78")
		require.ErrorIs(t, err, domain.ErrVinculoEnConflicto, "una cuenta, un identificador por partner")
		otra := nuevaFixture(t, db)
		_, err = admin.Vincular(ctx, actor, partner, usuarioDe(otra), "socio-77")
		require.ErrorIs(t, err, domain.ErrVinculoEnConflicto, "CA-10.4")
		_, err = admin.Vincular(ctx, actor, partner, uuid(), "socio-99")
		require.ErrorIs(t, err, domain.ErrCuentaNoVinculada)

		app := application.New(db, relojPrueba{time.Now().UTC().Truncate(time.Microsecond)}, ids)
		in := application.AcreditarPuntos{PartnerID: partner, ExternalUserID: "socio-77", Clave: "vinculo-0001", Puntos: 25, Motivo: "Compra vinculada", Referencia: "v-1", EventoID: "evento-vinculo-1"}
		_, err = app.Acreditar(ctx, in)
		require.NoError(t, err, "CA-10.3: el partner opera con su identificador")
		cuenta, err := admin.CuentaDeUsuario(ctx, actor, user, 1, 20)
		require.NoError(t, err)
		require.EqualValues(t, 25, cuenta.Saldo)
		require.Len(t, cuenta.Vinculaciones, 3)
	})

	t.Run("CA-11.1_11.2_reversion_idempotente_y_estado", func(t *testing.T) {
		f := nuevaFixture(t, db)
		user := usuarioDe(f)
		credito, err := f.app.Acreditar(ctx, f.credito("rev-credito-01", 10))
		require.NoError(t, err)
		original := credito.Resultado.Movimiento.Datos().ID
		res, repetida, err := admin.Revertir(ctx, actor, original, "Carga por error", "reversion-0001")
		require.NoError(t, err)
		require.False(t, repetida)
		require.Zero(t, res.Saldo)
		require.EqualValues(t, -10, res.Movimiento.Datos().Puntos)

		again, repetida, err := admin.Revertir(ctx, actor, original, "Carga por error", "reversion-0001")
		require.NoError(t, err)
		require.True(t, repetida)
		require.Equal(t, res.Movimiento.Datos().ID, again.Movimiento.Datos().ID)
		_, _, err = admin.Revertir(ctx, actor, original, "Otro motivo", "reversion-0001")
		require.ErrorIs(t, err, domain.ErrConflictoIdempotencia)
		_, _, err = admin.Revertir(ctx, actor, original, "Carga por error", "reversion-0002")
		require.ErrorIs(t, err, domain.ErrMovimientoRevertido, "CA-11.2")
		_, _, err = admin.Revertir(ctx, actor, res.Movimiento.Datos().ID, "Revertir la reversión", "reversion-0003")
		require.ErrorIs(t, err, domain.ErrMovimientoRevertido)
		_, _, err = admin.Revertir(ctx, actor, uuid(), "Inexistente", "reversion-0004")
		require.ErrorIs(t, err, domain.ErrMovimientoNoEncontrado)

		cuenta, err := admin.CuentaDeUsuario(ctx, actor, user, 1, 20)
		require.NoError(t, err)
		require.Zero(t, cuenta.Saldo)
		require.EqualValues(t, 2, cuenta.Pagina.Total)
		require.Equal(t, "REVERTIDO", cuenta.Pagina.EstadoMovimiento(original))
		require.Equal(t, res.Movimiento.Datos().ID, cuenta.Pagina.RevertidoPor[original])
		propia, err := application.NewBookingEvents(db, system.Clock{}, ids).Historial(ctx, user, 1, 20)
		require.NoError(t, err)
		require.Equal(t, domain.Reversion, propia.Movimientos[0].Datos().Tipo, "el alumno ve la reversión")
		partner, err := f.app.ListarMovimientos(ctx, f.partner, f.usuario, 1, 20)
		require.NoError(t, err)
		require.EqualValues(t, 1, partner.Total, "RN-26: el partner solo ve lo que originó")
		saldo, err := f.app.ConsultarSaldo(ctx, f.partner, f.usuario)
		require.NoError(t, err)
		require.Zero(t, saldo, "el saldo informado al partner incluye la reversión")
	})

	t.Run("CA-11.3_CL-13_saldo_insuficiente_y_CA-35.3", func(t *testing.T) {
		f := nuevaFixture(t, db)
		user := usuarioDe(f)
		credito, err := f.app.Acreditar(ctx, f.credito("rev-credito-02", 10))
		require.NoError(t, err)
		booking := application.NewBookingEvents(db, system.Clock{}, ids)
		reserva := uuid()
		penalizacion := domain.HechoBooking{EventoID: uuid(), Tipo: domain.PenalizacionInasistencia, AlumnoID: user, OrigenID: reserva, ReservaID: reserva, ClaseID: uuid(), OcurridoEn: time.Now().UTC().Truncate(time.Microsecond)}
		require.NoError(t, booking.Aplicar(ctx, penalizacion))
		_, _, err = admin.Revertir(ctx, actor, credito.Resultado.Movimiento.Datos().ID, "Carga por error", "reversion-0101")
		require.ErrorIs(t, err, domain.ErrSaldoInsuficiente)
		var huellas int
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT count(*) FROM reversiones_admin WHERE clave='reversion-0101'`).Scan(&huellas))
		require.Zero(t, huellas, "un rechazo no deja efectos")

		reserva2 := uuid()
		asistencia := domain.HechoBooking{EventoID: uuid(), Tipo: domain.Asistencia, AlumnoID: user, OrigenID: reserva2, ReservaID: reserva2, ClaseID: uuid(), OcurridoEn: time.Now().UTC().Truncate(time.Microsecond)}
		require.NoError(t, booking.Aplicar(ctx, asistencia))
		var asistenciaID string
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT movimiento_id::text FROM mensajes_booking_procesados WHERE event_id=$1`, asistencia.EventoID).Scan(&asistenciaID))
		res, _, err := admin.Revertir(ctx, actor, asistenciaID, "Ingreso registrado por error", "reversion-0102")
		require.NoError(t, err)
		require.Zero(t, res.Saldo)
		require.NoError(t, booking.Aplicar(ctx, asistencia), "la reentrega no vuelve a acreditar")
		var recibos int
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT count(*) FROM mensajes_booking_procesados WHERE origen_id=$1`, reserva2).Scan(&recibos))
		require.Equal(t, 1, recibos, "la asistencia y su recibo no cambian")
		cuenta, err := admin.CuentaDeUsuario(ctx, actor, user, 1, 20)
		require.NoError(t, err)
		require.Zero(t, cuenta.Saldo)
	})

	t.Run("reversiones_concurrentes_un_solo_compensatorio", func(t *testing.T) {
		f := nuevaFixture(t, db)
		credito, err := f.app.Acreditar(ctx, f.credito("rev-credito-03", 300))
		require.NoError(t, err)
		original := credito.Resultado.Movimiento.Datos().ID
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, _, err := admin.Revertir(ctx, actor, original, "Concurrencia", fmt.Sprintf("concurrente-%04d", i))
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
				require.ErrorIs(t, err, domain.ErrMovimientoRevertido)
			}
		}
		require.Equal(t, 1, ok)
		var saldo int64
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT saldo FROM cuentas WHERE id=$1`, f.cuenta).Scan(&saldo))
		require.Zero(t, saldo)
	})

	t.Run("base_rechaza_reversiones_incompatibles", func(t *testing.T) {
		f := nuevaFixture(t, db)
		credito, err := f.app.Acreditar(ctx, f.credito("rev-credito-04", 50))
		require.NoError(t, err)
		original := credito.Resultado.Movimiento.Datos().ID
		insertar := func(puntos int64, revierte string) error {
			tx, err := db.pool.Begin(ctx)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback(ctx) }()
			if _, err := tx.Exec(ctx, `INSERT INTO movimientos_puntos(id,cuenta_id,secuencia,puntos,motivo,referencia,ocurrido_en,aplicado_en,tipo,revierte_id) VALUES(gen_random_uuid(),$1,2,$2,'Manual','fixture',now(),now(),'REVERSION',$3)`, f.cuenta, puntos, revierte); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE cuentas SET saldo=saldo+$2,ultima_secuencia=2 WHERE id=$1`, f.cuenta, puntos); err != nil {
				return err
			}
			return tx.Commit(ctx)
		}
		require.Error(t, insertar(-40, original), "importe distinto del contrario")
		otra := nuevaFixture(t, db)
		ajeno, err := otra.app.Acreditar(ctx, otra.credito("rev-credito-05", 50))
		require.NoError(t, err)
		require.Error(t, insertar(-50, ajeno.Resultado.Movimiento.Datos().ID), "original de otra cuenta")
		require.NoError(t, insertar(-50, original))
		_, err = db.pool.Exec(ctx, `UPDATE movimientos_puntos SET puntos=1 WHERE id=$1`, original)
		require.Error(t, err, "RN-22: el original sigue inmutable")
	})
}
