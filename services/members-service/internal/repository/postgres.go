// Package repository es la capa de acceso a datos de members-service (members_db).
// Solo esta capa conoce PostgreSQL; la capa service depende de sus interfaces.
package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres envuelve el pool de conexiones a members_db.
type Postgres struct {
	pool *pgxpool.Pool
}

// NewPostgres crea el pool. No abre conexiones hasta el primer uso, así el servicio
// arranca aunque la base todavía no esté disponible (y /health/ready lo informa).
func NewPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("DSN de members_db inválido: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("crear pool de members_db: %w", err)
	}
	return &Postgres{pool: pool}, nil
}

// Ping verifica que members_db responde.
func (p *Postgres) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

// Close libera las conexiones.
func (p *Postgres) Close() {
	p.pool.Close()
}
