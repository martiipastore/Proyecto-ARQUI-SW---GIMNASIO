package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestDB_PingContraBaseReal levanta un PostgreSQL 16 real con testcontainers.
// Se saltea si Docker no está disponible.
func TestDB_PingContraBaseReal(t *testing.T) {
	requireDocker(t)
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("benefits_db"),
		tcpostgres.WithUsername("benefits_user"),
		tcpostgres.WithPassword("test-password"),
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

	db, err := Open(ctx, dsn)
	require.NoError(t, err)
	defer db.Close()

	assert.NoError(t, db.Ping(ctx))
}

func TestDB_PingFallaSiLaBaseNoResponde(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := Open(ctx, "postgres://benefits_user:x@127.0.0.1:1/benefits_db?sslmode=disable&connect_timeout=1")
	require.NoError(t, err)
	defer db.Close()

	assert.Error(t, db.Ping(ctx))
}

// En CI y en la validación de etapa Docker es obligatorio. El modo local común
// conserva el skip explícito, sin confundirlo con evidencia de integración.
func requireDocker(t *testing.T) {
	t.Helper()
	mandatory := os.Getenv("REQUIRE_DOCKER_TESTS") == "1" || os.Getenv("CI") == "true"
	unavailable := func(reason any) {
		if mandatory {
			t.Fatalf("Docker obligatorio para integración: %v", reason)
		}
		t.Skipf("Docker no disponible: %v", reason)
	}
	defer func() {
		if r := recover(); r != nil {
			unavailable(r)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	provider, err := testcontainers.ProviderDocker.GetProvider()
	if err != nil {
		unavailable(err)
		return
	}
	if err := provider.Health(ctx); err != nil {
		unavailable(err)
	}
}
