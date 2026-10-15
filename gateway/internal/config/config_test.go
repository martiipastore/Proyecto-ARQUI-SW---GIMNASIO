package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func route(t *testing.T, cfg Config, prefix string) Route {
	t.Helper()
	for _, r := range cfg.Routes {
		if r.Prefix == prefix {
			return r
		}
	}
	t.Fatalf("no existe la ruta %s", prefix)
	return Route{}
}

func TestLoad_ValoresPorDefecto(t *testing.T) {
	cfg, err := LoadFromMap(nil)
	require.NoError(t, err)

	assert.Equal(t, ":8080", cfg.HTTPAddr)
	assert.Len(t, cfg.Services, 6)
	assert.Equal(t, Members, route(t, cfg, "/api/v1/auth").Service)
	assert.Equal(t, Booking, route(t, cfg, "/api/v1/bookings").Service)
	assert.Equal(t, Benefits, route(t, cfg, "/partner-api/v1").Service)
	assert.Equal(t, 3*time.Second, route(t, cfg, "/api/v1/users").Timeout)
	assert.Equal(t, 5*time.Second, route(t, cfg, "/api/v1/bookings").Timeout)
}

func TestLoad_TimeoutPorRutaConfigurable(t *testing.T) {
	cfg, err := LoadFromMap(map[string]string{
		"GATEWAY_TIMEOUT_DEFAULT":  "1s",
		"GATEWAY_TIMEOUT_BOOKINGS": "750ms",
	})
	require.NoError(t, err)
	assert.Equal(t, time.Second, route(t, cfg, "/api/v1/classes").Timeout)
	assert.Equal(t, 750*time.Millisecond, route(t, cfg, "/api/v1/bookings").Timeout)
}

func TestLoad_RutasOrdenadasPorPrefijoMasLargo(t *testing.T) {
	cfg, err := LoadFromMap(nil)
	require.NoError(t, err)
	for i := 1; i < len(cfg.Routes); i++ {
		assert.GreaterOrEqual(t, len(cfg.Routes[i-1].Prefix), len(cfg.Routes[i].Prefix))
	}
}

func TestLoad_URLInvalida(t *testing.T) {
	_, err := LoadFromMap(map[string]string{"GATEWAY_MEMBERS_URL": "members-service"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GATEWAY_MEMBERS_URL")
}

func TestLoad_TimeoutInvalido(t *testing.T) {
	_, err := LoadFromMap(map[string]string{"GATEWAY_TIMEOUT_USERS": "rapido"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GATEWAY_TIMEOUT_USERS")
}
