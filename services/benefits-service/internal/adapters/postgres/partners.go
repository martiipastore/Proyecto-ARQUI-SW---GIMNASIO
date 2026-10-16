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
	err = d.pool.QueryRow(ctx, `SELECT id::text,activo FROM partners WHERE api_key_hash=$1`, hash[:]).Scan(&p.ID, &p.Activo)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, nil
	}
	return p, storageError(err)
}
