package postgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/migrations"
	"github.com/jackc/pgx/v5"
)

// Migrate aplica cada archivo una sola vez, comprueba su huella y conserva
// el SQL y su versión en la misma transacción. No realiza cambios destructivos.
func (d *DB) Migrate(ctx context.Context) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		// La limpieza es intencional: tras Commit, Rollback devuelve ErrTxClosed.
		_ = tx.Rollback(context.WithoutCancel(ctx))
	}()
	if _, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, huella text NOT NULL, aplicada_en timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `LOCK TABLE schema_migrations IN EXCLUSIVE MODE`); err != nil {
		return err
	}
	files, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		return err
	}
	for _, name := range files {
		contents, err := migrations.Files.ReadFile(name)
		if err != nil {
			return err
		}
		hash := fmt.Sprintf("%x", sha256.Sum256(contents))
		var old string
		err = tx.QueryRow(ctx, `SELECT huella FROM schema_migrations WHERE version=$1`, name).Scan(&old)
		if err == nil {
			if old != hash {
				return fmt.Errorf("migración %s modificada después de aplicarse", name)
			}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err = tx.Exec(ctx, string(contents)); err != nil {
			return fmt.Errorf("aplicar %s: %w", name, err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO schema_migrations(version, huella) VALUES ($1,$2)`, name, hash); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
