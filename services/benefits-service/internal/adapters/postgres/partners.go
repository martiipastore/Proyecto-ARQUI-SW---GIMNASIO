package postgres

import (
	"context"
	"errors"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/jackc/pgx/v5"
)

func (d *DB) BuscarPartner(ctx context.Context, hash [32]byte) (partner domain.Partner, err error) {
	ctx, finish := startDB(ctx, "postgres.autenticar_partner")
	defer func() { finish(err) }()
	var p domain.Partner
	var max int
	err = d.pool.QueryRow(ctx, `SELECT id::text,activo,max_solicitudes_minuto FROM partners WHERE api_key_hash=$1`, hash[:]).Scan(&p.ID, &p.Activo, &max)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, nil
	}
	if err == nil && p.Activo {
		var count int
		err = d.pool.QueryRow(ctx, `INSERT INTO solicitudes_integracion(partner_id,ventana,cantidad) VALUES($1,date_trunc('minute',now()),1) ON CONFLICT(partner_id,ventana) DO UPDATE SET cantidad=LEAST(solicitudes_integracion.cantidad+1,$2+1) RETURNING cantidad`, p.ID, max).Scan(&count)
		if err == nil && count > max {
			return p, domain.ErrLimiteSolicitudes
		}
	}
	return p, storageError(err)
}
