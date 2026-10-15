package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestPostgres_PingContraBaseReal levanta un PostgreSQL 16 real con testcontainers.
// Se saltea si Docker no está disponible.
func TestPostgres_PingContraBaseReal(t *testing.T) {
	requireDocker(t)
	ctx := context.Background()

	ctr, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("members_db"),
		postgres.WithUsername("members_user"),
		postgres.WithPassword("test-password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(90*time.Second)),
	)
	t.Cleanup(func() {
		if ctr != nil {
			_ = ctr.Terminate(context.Background())
		}
	})
	require.NoError(t, err)

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := NewPostgres(ctx, dsn)
	require.NoError(t, err)
	defer db.Close()

	assert.NoError(t, db.Ping(ctx))
}

func TestPostgres_PingFallaSiLaBaseNoResponde(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := NewPostgres(ctx, "postgres://members_user:x@127.0.0.1:1/members_db?sslmode=disable&connect_timeout=1")
	require.NoError(t, err, "crear el pool no debe conectarse todavía")
	defer db.Close()

	assert.Error(t, db.Ping(ctx))
}

func TestNewPostgres_DSNInvalido(t *testing.T) {
	_, err := NewPostgres(context.Background(), "esto no es un dsn ://")
	assert.Error(t, err)
}

// requireDocker saltea el test si Docker no está disponible. En testcontainers 0.34,
// SkipIfProviderIsNotHealthy entra en pánico (en lugar de saltear) si no encuentra
// el daemon; acá ese pánico se convierte en un skip.
func requireDocker(t *testing.T) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Skipf("Docker no disponible: %v", r)
		}
	}()
	testcontainers.SkipIfProviderIsNotHealthy(t)
}
