// Etapa 12: control de tráfico del gateway (ARCHITECTURE §5.1 paso 7, §5.2 y
// §5.3). Store en memoria para aislar las reglas del gateway; el script de Redis
// se prueba contra Redis real en internal/ratelimit.
package server

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/proxy"
	"github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/ratelimit"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/access"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/logger"
)

// memStore permite Burst solicitudes por clave y después rechaza; con down, falla.
type memStore struct {
	mu    sync.Mutex
	count map[string]int
	keys  []string
	down  bool
}

func (s *memStore) Take(_ context.Context, key string, p ratelimit.Policy) (ratelimit.Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.down {
		return ratelimit.Decision{}, errors.New("redis caído")
	}
	s.keys = append(s.keys, key)
	s.count[key]++
	if s.count[key] > p.Burst {
		return ratelimit.Decision{Allowed: false, RetryAfter: 1500 * time.Millisecond}, nil
	}
	return ratelimit.Decision{Allowed: true}, nil
}

func limitedFixture(t *testing.T) (*fixture, *memStore, *ratelimit.Limiter) {
	t.Helper()
	store := &memStore{count: map[string]int{}}
	l := proxy.Limits{
		Login:   ratelimit.Policy{Name: "login", Rate: 5.0 / 60, Burst: 5, FailClosed: true},
		User:    ratelimit.Policy{Name: "user", Rate: 10, Burst: 20},
		Partner: ratelimit.Policy{Name: "partner", Rate: 20, Burst: 40},
	}
	l.Limiter = ratelimit.New(store, logger.NewWithWriter(&bytes.Buffer{}, "api-gateway", "error"), l.Login, l.User, l.Partner)
	return newFixture(t, nil, l), store, l.Limiter
}

func TestRateLimit_LoginCincoPorMinutoPorIP(t *testing.T) {
	f, _, _ := limitedFixture(t)
	for i := 0; i < 5; i++ {
		require.Equal(t, http.StatusOK, f.do(http.MethodPost, "/api/v1/auth/login", nil).Code, "intento %d", i+1)
	}
	rec := f.do(http.MethodPost, "/api/v1/auth/login", nil)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Equal(t, "2", rec.Header().Get("Retry-After"), "redondea 1,5 s hacia arriba")
	require.Contains(t, rec.Header().Get("Content-Type"), "application/problem+json")
	require.Contains(t, rec.Body.String(), `"code":"LIMITE_EXCEDIDO"`)
	require.Equal(t, 5, len(f.upstreams["members-service"].paths), "el intento rechazado no llega a members")
}

func TestRateLimit_UsuarioPorSubVerificadoNoPorCabeceraDelCliente(t *testing.T) {
	f, store, _ := limitedFixture(t)
	h := adminHeaders(t)
	h["X-User-Id"] = "identidad-falsa"
	for i := 0; i < 20; i++ {
		require.Equal(t, http.StatusOK, f.do(http.MethodGet, "/api/v1/users", h).Code)
	}
	rec := f.do(http.MethodGet, "/api/v1/users", h)
	require.Equal(t, http.StatusTooManyRequests, rec.Code, "ráfaga de 20 agotada")
	require.Contains(t, rec.Body.String(), "LIMITE_EXCEDIDO")
	for _, k := range store.keys {
		require.Equal(t, "gw:rl:user:user:11111111-1111-4111-8111-111111111111", k, "la clave sale del JWT, no de X-User-Id")
	}
}

func TestRateLimit_UsuariosDistintosNoComparten(t *testing.T) {
	f, store, _ := limitedFixture(t)
	store.count["gw:rl:user:user:11111111-1111-4111-8111-111111111111"] = 20
	require.Equal(t, http.StatusTooManyRequests, f.do(http.MethodGet, "/api/v1/users", adminHeaders(t)).Code)
	// Otra política (login, por IP) sigue con su propio balde.
	require.Equal(t, http.StatusOK, f.do(http.MethodPost, "/api/v1/auth/login", nil).Code)
}

func TestRateLimit_PartnerPorHashDeAPIKeyConFormatoDelContrato(t *testing.T) {
	f, store, _ := limitedFixture(t)
	h := map[string]string{"X-API-Key": "clave-secreta-del-partner"}
	for i := 0; i < 40; i++ {
		require.Equal(t, http.StatusOK, f.do(http.MethodGet, "/partner-api/v2/accounts/u1/balance", h).Code)
	}
	rec := f.do(http.MethodGet, "/partner-api/v2/accounts/u1/balance", h)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.NotEmpty(t, rec.Header().Get("Retry-After"))
	require.Contains(t, rec.Body.String(), "RATE_LIMITED", "código del contrato de partners v2")
	for _, k := range store.keys {
		require.NotContains(t, k, "clave-secreta", "la API key nunca se guarda en claro")
		require.True(t, strings.HasPrefix(k, "gw:rl:partner:key:"))
	}
	// Otra key tiene su propio balde.
	require.Equal(t, http.StatusOK, f.do(http.MethodGet, "/partner-api/v2/accounts/u1/balance", map[string]string{"X-API-Key": "otra"}).Code)
}

func TestRateLimit_RedisCaidoLoginFallaCerradoYElRestoAbierto(t *testing.T) {
	f, store, limiter := limitedFixture(t)
	store.down = true
	rec := f.do(http.MethodPost, "/api/v1/auth/login", nil)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code, "login falla cerrado (§5.3)")
	require.Contains(t, rec.Body.String(), "DEPENDENCIA_NO_DISPONIBLE")
	require.NotEmpty(t, rec.Header().Get("Retry-After"))
	require.Equal(t, http.StatusOK, f.do(http.MethodGet, "/api/v1/users", adminHeaders(t)).Code, "usuarios fallan abierto")
	require.Equal(t, http.StatusOK, f.do(http.MethodGet, "/partner-api/v2/accounts/u1/balance", map[string]string{"X-API-Key": "k"}).Code, "partners fallan abierto")
	require.Equal(t, int64(3), limiter.Metrics.RedisErrors.Load())
}

func TestRateLimit_SinLimiteEnRutasPropiasYMetricas(t *testing.T) {
	f, store, _ := limitedFixture(t)
	for i := 0; i < 30; i++ {
		require.NotEqual(t, http.StatusTooManyRequests, f.do(http.MethodGet, "/api/v1/status", nil).Code)
		require.Equal(t, http.StatusOK, f.do(http.MethodGet, "/health/live", nil).Code)
	}
	require.Empty(t, store.keys, "status y health no consumen fichas")
	f.do(http.MethodPost, "/api/v1/auth/login", nil)
	store.count["gw:rl:user:user:11111111-1111-4111-8111-111111111111"] = 20
	f.do(http.MethodGet, "/api/v1/users", map[string]string{"Authorization": "Bearer " + testJWT(t, access.Admin, time.Now())})
	body := f.do(http.MethodGet, "/metrics", nil).Body.String()
	require.Contains(t, body, `gateway_ratelimit_allowed_total{policy="login"} 1`)
	require.Contains(t, body, `gateway_ratelimit_rejected_total{policy="user"} 1`)
	require.Contains(t, body, "ratelimit_redis_errors_total 0")
}

func TestRateLimit_SinAutenticarNoConsumeFichas(t *testing.T) {
	f, store, _ := limitedFixture(t)
	require.Equal(t, http.StatusUnauthorized, f.do(http.MethodGet, "/api/v1/users", nil).Code)
	require.Empty(t, store.keys, "el 401 se responde antes del límite")
}
