// Package postgres es el adaptador de salida de booking-service hacia booking_db.
// Implementa los puertos Catalogo y Reservas de internal/ports.
package postgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/migrations"
)

// DB envuelve el pool de conexiones a booking_db.
type DB struct {
	pool *pgxpool.Pool
}

// Open crea el pool. No abre conexiones hasta el primer uso, así el servicio
// arranca aunque la base todavía no esté disponible (y /health/ready lo informa).
func Open(ctx context.Context, dsn string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("DSN de booking_db inválido: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("crear pool de booking_db: %w", err)
	}
	return &DB{pool: pool}, nil
}

// Ping verifica que booking_db responde.
func (d *DB) Ping(ctx context.Context) error {
	return d.pool.Ping(ctx)
}

// Close libera las conexiones.
func (d *DB) Close() {
	d.pool.Close()
}

// Migrate aplica en orden las migraciones embebidas que falten. Rechaza una
// migración ya aplicada cuyo contenido cambió. Varias instancias se serializan.
func (d *DB) Migrate(ctx context.Context) error {
	return d.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(620261109)`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version text PRIMARY KEY, huella text NOT NULL)`); err != nil {
			return err
		}
		files, err := fs.Glob(migrations.Files, "*.sql")
		if err != nil {
			return err
		}
		for _, file := range files {
			data, err := migrations.Files.ReadFile(file)
			if err != nil {
				return err
			}
			hash := fmt.Sprintf("%x", sha256.Sum256(data))
			var old string
			err = tx.QueryRow(ctx, `SELECT huella FROM schema_migrations WHERE version=$1`, file).Scan(&old)
			if err == nil {
				if old != hash {
					return fmt.Errorf("migración modificada: %s", file)
				}
				continue
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if _, err = tx.Exec(ctx, string(data)); err != nil {
				return fmt.Errorf("aplicar %s: %w", file, err)
			}
			if _, err = tx.Exec(ctx, `INSERT INTO schema_migrations VALUES ($1,$2)`, file, hash); err != nil {
				return err
			}
		}
		return nil
	})
}

func (d *DB) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// traducir convierte errores de la base en errores de dominio. Las violaciones
// de restricciones son la última defensa de las invariantes (regla 8): si el
// código las dejara pasar, la base igual las rechaza con el mismo motivo.
func traducir(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNoEncontrado
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.ConstraintName {
		case "reserva_activa_unica":
			return domain.ErrReservaDuplicada
		case "musculacion_un_turno_por_dia":
			return domain.ErrTurnoMusculacionDiario
		case "alumno_sin_reservas_superpuestas":
			return domain.ErrSuperposicionReserva
		case "cupo":
			return domain.ErrSinCupo
		case "profesor_sin_horarios_superpuestos", "profesor_sin_clases_superpuestas":
			return domain.ErrSuperposicionProfesor
		case "actividades_nombre_unico", "tope_actividad":
			return domain.ErrValidacion
		}
	}
	return err
}

type consultor interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func auditar(ctx context.Context, tx pgx.Tx, actor, accion, entidad, id, detalle string) error {
	_, err := tx.Exec(ctx, `INSERT INTO auditoria(actor_id,accion,entidad,entidad_id,detalle) VALUES(NULLIF($1,'')::uuid,$2,$3,NULLIF($4,'')::uuid,NULLIF($5,''))`, actor, accion, entidad, id, detalle)
	return err
}
