package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/ports"
)

var (
	_ ports.ReservasDelAlumno = (*DB)(nil)
	_ ports.FuenteClases      = (*DB)(nil)
)

// ReservadasPor marca las clases de una página que el alumno tiene CONFIRMADA
// (CA-13.4). Es una consulta acotada a los ids de la página.
func (d *DB) ReservadasPor(ctx context.Context, alumnoID string, claseIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	rows, err := d.pool.Query(ctx, `SELECT clase_id::text FROM reservas WHERE alumno_id=$1 AND estado='CONFIRMADA' AND clase_id = ANY($2::uuid[])`, alumnoID, claseIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// RecorrerClases lee todas las clases por id en lotes (keyset), cada lote con
// su versión actual, para reconstruir el índice (ADR-014).
func (d *DB) RecorrerClases(ctx context.Context, lote int, fn func([]domain.Clase) error) error {
	ultimo := "00000000-0000-0000-0000-000000000000"
	for {
		rows, err := d.pool.Query(ctx, `SELECT `+claseCols+claseFrom+` WHERE c.id > $1::uuid ORDER BY c.id LIMIT $2`, ultimo, lote)
		if err != nil {
			return err
		}
		clases, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Clase, error) {
			var c domain.Clase
			return c, row.Scan(claseDestinos(&c)...)
		})
		if err != nil {
			return err
		}
		if len(clases) == 0 {
			return nil
		}
		if err := fn(clases); err != nil {
			return err
		}
		ultimo = clases[len(clases)-1].ID
		if len(clases) < lote {
			return nil
		}
	}
}

// ResumenClases es la huella de convergencia: cantidad y suma de versiones.
func (d *DB) ResumenClases(ctx context.Context) (cantidad, sumaVersiones int64, err error) {
	err = d.pool.QueryRow(ctx, `SELECT count(*), COALESCE(sum(version),0)::bigint FROM clases`).Scan(&cantidad, &sumaVersiones)
	return cantidad, sumaVersiones, err
}
