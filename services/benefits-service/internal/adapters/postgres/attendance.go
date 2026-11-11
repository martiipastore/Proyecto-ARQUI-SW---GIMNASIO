package postgres

import (
	"context"
	"errors"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/jackc/pgx/v5"
)

// AplicarHecho confirma movimiento, cuenta y recibo en la misma transacción.
// El callback aplica las reglas del dominio sobre el saldo bloqueado. Todos los
// créditos HTTP y eventos comparten SELECT FOR UPDATE sobre la misma cuenta.
func (d *DB) AplicarHecho(ctx context.Context, h domain.HechoBooking, fingerprint string, apply func(domain.CuentaBeneficios) (domain.CuentaBeneficios, domain.MovimientoPuntos, error), envio *domain.EnvioMelt) (err error) {
	defer func() { err = storageError(err) }()
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "booking:"+h.EventoID); err != nil {
		return err
	}
	var old string
	err = tx.QueryRow(ctx, `SELECT huella FROM mensajes_booking_procesados WHERE event_id=$1`, h.EventoID).Scan(&old)
	if err == nil {
		if old != fingerprint {
			return domain.ErrConflictoIdempotencia
		}
		return tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO cuentas(id,alumno_id) VALUES(gen_random_uuid(),$1) ON CONFLICT(alumno_id) DO NOTHING`, h.AlumnoID); err != nil {
		return err
	}
	var id string
	var saldo, seq int64
	if err = tx.QueryRow(ctx, `SELECT id::text,saldo,ultima_secuencia FROM cuentas WHERE alumno_id=$1 FOR UPDATE`, h.AlumnoID).Scan(&id, &saldo, &seq); err != nil {
		return err
	}
	var movement string
	err = tx.QueryRow(ctx, `SELECT movimiento_id::text,huella FROM mensajes_booking_procesados WHERE tipo=$1 AND origen_id=$2 LIMIT 1`, h.Tipo, h.OrigenID).Scan(&movement, &old)
	if err == nil {
		if old != fingerprint {
			return domain.ErrConflictoIdempotencia
		}
	} else {
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		cuenta, err := domain.NuevaCuenta(id, saldo, seq)
		if err != nil {
			return err
		}
		nueva, mov, err := apply(cuenta)
		if err != nil {
			return err
		}
		m := mov.Datos()
		movement = m.ID
		if _, err = tx.Exec(ctx, `INSERT INTO movimientos_puntos(id,cuenta_id,secuencia,puntos,motivo,referencia,ocurrido_en,aplicado_en,tipo,origen_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, m.ID, m.CuentaID, m.Secuencia, m.Puntos, m.Motivo, m.Referencia, m.OcurridoEn, m.AplicadoEn, m.Tipo, m.OrigenID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE cuentas SET saldo=$2,ultima_secuencia=$3 WHERE id=$1`, nueva.ID(), nueva.Saldo(), nueva.Secuencia()); err != nil {
			return err
		}
		// Etapa 13: el envío a MELT nace en la misma transacción que la
		// asistencia; una reentrega del evento no lo duplica (origen único).
		if envio != nil {
			if err = insertarEnvioMelt(ctx, tx, *envio); err != nil {
				return err
			}
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO mensajes_booking_procesados(event_id,tipo,origen_id,huella,movimiento_id) VALUES($1,$2,$3,$4,$5)`, h.EventoID, h.Tipo, h.OrigenID, fingerprint, movement); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (d *DB) HistorialUsuario(ctx context.Context, user string, page, size int) (out domain.PaginaMovimientos, err error) {
	defer func() { err = storageError(err) }()
	out.Movimientos = []domain.MovimientoPuntos{}
	tx, err := d.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	var account string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM cuentas WHERE alumno_id=$1`, user).Scan(&account); errors.Is(err, pgx.ErrNoRows) {
		return out, domain.ErrCuentaNoVinculada
	} else if err != nil {
		return out, err
	}
	// Etapa 7: el historial propio también muestra reversiones y su estado.
	if out, err = paginaCuenta(ctx, tx, account, page, size); err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
