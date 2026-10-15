// Package postgres es el adaptador de salida de booking-service hacia booking_db.
// Implementará los puertos de repositorio de internal/ports.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
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
