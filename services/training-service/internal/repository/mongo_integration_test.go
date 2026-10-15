package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
)

// TestMongo_PingContraBaseReal levanta un MongoDB 7 real con testcontainers.
// Se saltea si Docker no está disponible.
func TestMongo_PingContraBaseReal(t *testing.T) {
	requireDocker(t)
	ctx := context.Background()

	ctr, err := mongodb.Run(ctx, "mongo:7")
	t.Cleanup(func() {
		if ctr != nil {
			_ = ctr.Terminate(context.Background())
		}
	})
	require.NoError(t, err)

	uri, err := ctr.ConnectionString(ctx)
	require.NoError(t, err)

	m, err := NewMongo(uri, "training_db")
	require.NoError(t, err)
	defer func() { _ = m.Close(context.Background()) }()

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	assert.NoError(t, m.Ping(pingCtx))
	assert.Equal(t, "training_db", m.Database().Name())
}

func TestMongo_PingFallaSiLaBaseNoResponde(t *testing.T) {
	m, err := NewMongo("mongodb://127.0.0.1:1/?serverSelectionTimeoutMS=500", "training_db")
	require.NoError(t, err, "crear el cliente no debe conectarse todavía")
	defer func() { _ = m.Close(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	assert.Error(t, m.Ping(ctx))
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
