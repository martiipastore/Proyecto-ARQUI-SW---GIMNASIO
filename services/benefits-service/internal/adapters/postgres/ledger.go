package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/ports"
	"github.com/jackc/pgx/v5"
)

var _ ports.Ledger = (*DB)(nil)

type transaction struct{ tx pgx.Tx }

// snapshot pertenece al adaptador: el dominio no conoce JSON ni pgx.
type snapshot struct {
	Movimiento domain.DatosMovimiento `json:"movimiento"`
	Saldo      int64                  `json:"saldo"`
	Rechazo    string                 `json:"rechazo,omitempty"`
}

func (d *DB) EnTransaccion(ctx context.Context, fn func(ports.Transaction) error) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	if err = fn(&transaction{tx: tx}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (t *transaction) PartnerActivo(ctx context.Context, id string) (bool, error) {
	var activo bool
	err := t.tx.QueryRow(ctx, `SELECT activo FROM partners WHERE id=$1 FOR SHARE`, id).Scan(&activo)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return activo, err
}

func (t *transaction) TomarOperacion(ctx context.Context, partner, clave, huella string, ahora time.Time) (ports.OperacionGuardada, error) {
	tag, err := t.tx.Exec(ctx, `INSERT INTO operaciones_partner(partner_id,clave,huella,creada_en,conservar_hasta) VALUES ($1,$2,$3,$4,$4::timestamptz+interval '30 days') ON CONFLICT DO NOTHING`, partner, clave, huella, ahora)
	if err != nil {
		return ports.OperacionGuardada{}, err
	}
	if tag.RowsAffected() == 1 {
		return ports.OperacionGuardada{Nueva: true, Huella: huella}, nil
	}
	var contenido []byte
	var guardada ports.OperacionGuardada
	err = t.tx.QueryRow(ctx, `SELECT huella,resultado FROM operaciones_partner WHERE partner_id=$1 AND clave=$2 FOR UPDATE`, partner, clave).Scan(&guardada.Huella, &contenido)
	if err != nil {
		return guardada, err
	}
	var s snapshot
	if err = json.Unmarshal(contenido, &s); err != nil {
		return guardada, err
	}
	guardada.Resultado = domain.ResultadoOperacion{Saldo: s.Saldo, Rechazo: s.Rechazo}
	if s.Rechazo == "" {
		m, err := domain.NuevoMovimiento(s.Movimiento)
		if err != nil {
			return guardada, err
		}
		guardada.Resultado.Movimiento = m
	}
	return guardada, nil
}

func (t *transaction) CuentaVinculada(ctx context.Context, partner, usuario string) (domain.CuentaBeneficios, error) {
	var id string
	var saldo, secuencia int64
	err := t.tx.QueryRow(ctx, `SELECT c.id::text,c.saldo,c.ultima_secuencia FROM cuentas c JOIN vinculaciones v ON v.cuenta_id=c.id WHERE v.partner_id=$1 AND v.external_user_id=$2 FOR UPDATE OF c`, partner, usuario).Scan(&id, &saldo, &secuencia)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CuentaBeneficios{}, domain.ErrCuentaNoVinculada
	}
	if err != nil {
		return domain.CuentaBeneficios{}, err
	}
	return domain.NuevaCuenta(id, saldo, secuencia)
}

func (t *transaction) GuardarMovimiento(ctx context.Context, cuenta domain.CuentaBeneficios, movimiento domain.MovimientoPuntos) error {
	m := movimiento.Datos()
	_, err := t.tx.Exec(ctx, `INSERT INTO movimientos_puntos(id,cuenta_id,partner_id,secuencia,puntos,motivo,referencia,ocurrido_en,aplicado_en) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, m.ID, m.CuentaID, m.PartnerID, m.Secuencia, m.Puntos, m.Motivo, m.Referencia, m.OcurridoEn, m.AplicadoEn)
	if err != nil {
		return err
	}
	_, err = t.tx.Exec(ctx, `UPDATE cuentas SET saldo=$2,ultima_secuencia=$3 WHERE id=$1`, cuenta.ID(), cuenta.Saldo(), cuenta.Secuencia())
	return err
}

func (t *transaction) CompletarOperacion(ctx context.Context, partner, clave string, resultado domain.ResultadoOperacion) error {
	contents, err := json.Marshal(snapshot{Movimiento: resultado.Movimiento.Datos(), Saldo: resultado.Saldo, Rechazo: resultado.Rechazo})
	if err != nil {
		return err
	}
	_, err = t.tx.Exec(ctx, `UPDATE operaciones_partner SET resultado=$3 WHERE partner_id=$1 AND clave=$2`, partner, clave, contents)
	return err
}

func (d *DB) ConsultarSaldo(ctx context.Context, partner, usuario string) (ports.SaldoConsultado, error) {
	var r ports.SaldoConsultado
	err := d.EnTransaccion(ctx, func(raw ports.Transaction) error {
		t := raw.(*transaction)
		activo, err := t.PartnerActivo(ctx, partner)
		if err != nil {
			return err
		}
		r.PartnerActivo = activo
		if !activo {
			return nil
		}
		err = t.tx.QueryRow(ctx, `SELECT c.saldo FROM cuentas c JOIN vinculaciones v ON v.cuenta_id=c.id WHERE v.partner_id=$1 AND v.external_user_id=$2`, partner, usuario).Scan(&r.Saldo)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		r.CuentaVinculada = err == nil
		return err
	})
	return r, err
}

func (d *DB) ListarMovimientos(ctx context.Context, partner, usuario string, pagina, tamanio int) (ports.MovimientosConsultados, error) {
	r := ports.MovimientosConsultados{Pagina: domain.PaginaMovimientos{Movimientos: []domain.MovimientoPuntos{}}}
	tx, err := d.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return r, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	t := &transaction{tx: tx}
	r.PartnerActivo, err = t.PartnerActivo(ctx, partner)
	if err != nil {
		return r, err
	}
	if !r.PartnerActivo {
		return r, tx.Commit(ctx)
	}
	var cuenta string
	err = tx.QueryRow(ctx, `SELECT cuenta_id::text FROM vinculaciones WHERE partner_id=$1 AND external_user_id=$2`, partner, usuario).Scan(&cuenta)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, tx.Commit(ctx)
	}
	if err != nil {
		return r, err
	}
	r.CuentaVinculada = true
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM movimientos_puntos WHERE cuenta_id=$1 AND partner_id=$2`, cuenta, partner).Scan(&r.Pagina.Total); err != nil {
		return r, err
	}
	rows, err := tx.Query(ctx, `SELECT id::text,cuenta_id::text,partner_id::text,puntos,secuencia,motivo,referencia,ocurrido_en,aplicado_en FROM movimientos_puntos WHERE cuenta_id=$1 AND partner_id=$2 ORDER BY ocurrido_en DESC,secuencia DESC LIMIT $3 OFFSET $4`, cuenta, partner, tamanio, int64(pagina-1)*int64(tamanio))
	if err != nil {
		return r, err
	}
	defer rows.Close()
	for rows.Next() {
		var m domain.DatosMovimiento
		if err = rows.Scan(&m.ID, &m.CuentaID, &m.PartnerID, &m.Puntos, &m.Secuencia, &m.Motivo, &m.Referencia, &m.OcurridoEn, &m.AplicadoEn); err != nil {
			return r, err
		}
		movimiento, err := domain.NuevoMovimiento(m)
		if err != nil {
			return r, err
		}
		r.Pagina.Movimientos = append(r.Pagina.Movimientos, movimiento)
	}
	if err = rows.Err(); err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}
