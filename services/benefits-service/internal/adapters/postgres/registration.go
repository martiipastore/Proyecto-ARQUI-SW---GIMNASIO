package postgres

import (
	"context"
	"errors"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/system"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/ports"
	"github.com/jackc/pgx/v5"
)

func (d *DB) SaldoUsuario(ctx context.Context, user string) (ports.SaldoConsultado, error) {
	var out ports.SaldoConsultado
	err := d.pool.QueryRow(ctx, `SELECT c.saldo,COALESCE((SELECT max(aplicado_en) FROM movimientos_puntos WHERE cuenta_id=c.id),c.creada_en) FROM cuentas c WHERE c.alumno_id=$1`, user).Scan(&out.Saldo, &out.ActualizadoEn)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, domain.ErrCuentaNoVinculada
	}
	return out, storageError(err)
}

func (d *DB) PoliticaRegistro(ctx context.Context, partner string, points int64) (string, error) {
	var name string
	var active, registration, credits bool
	var max int64
	err := d.pool.QueryRow(ctx, `SELECT nombre,activo,permite_registro,permite_puntos,max_puntos_operacion FROM partners WHERE id=$1`, partner).Scan(&name, &active, &registration, &credits, &max)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrPartnerNoAutorizado
	}
	if err != nil {
		return "", storageError(err)
	}
	if !active || !registration || (points > 0 && !credits) {
		return "", domain.ErrPartnerNoAutorizado
	}
	if points > max {
		return "", domain.ErrOperacionInvalida
	}
	return name, nil
}

func (d *DB) VincularExterno(ctx context.Context, partner, external, user string) (int64, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return 0, storageError(err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	var active bool
	if err = tx.QueryRow(ctx, `SELECT activo AND permite_registro FROM partners WHERE id=$1 FOR SHARE`, partner).Scan(&active); err != nil {
		return 0, storageError(err)
	}
	if !active {
		return 0, domain.ErrPartnerNoAutorizado
	}
	id, err := (system.IDs{}).NewID()
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO cuentas(id,alumno_id) VALUES($1,$2) ON CONFLICT(alumno_id) DO NOTHING`, id, user); err != nil {
		return 0, storageError(err)
	}
	var balance int64
	if err = tx.QueryRow(ctx, `SELECT id::text,saldo FROM cuentas WHERE alumno_id=$1 FOR UPDATE`, user).Scan(&id, &balance); err != nil {
		return 0, storageError(err)
	}
	// Ambos índices de unicidad se verifican antes de insertar; nunca se reasigna.
	var existing string
	err = tx.QueryRow(ctx, `SELECT cuenta_id::text FROM vinculaciones WHERE partner_id=$1 AND (external_user_id=$2 OR cuenta_id=$3)`, partner, external, id).Scan(&existing)
	if err == nil {
		var matches bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vinculaciones WHERE partner_id=$1 AND external_user_id=$2 AND cuenta_id=$3)`, partner, external, id).Scan(&matches); err != nil {
			return 0, err
		}
		if !matches {
			return 0, domain.ErrConflictoIdempotencia
		}
	} else if errors.Is(err, pgx.ErrNoRows) {
		if _, err = tx.Exec(ctx, `INSERT INTO vinculaciones(partner_id,external_user_id,cuenta_id) VALUES($1,$2,$3)`, partner, external, id); err != nil {
			return 0, storageError(err)
		}
	} else {
		return 0, storageError(err)
	}
	return balance, storageError(tx.Commit(ctx))
}
