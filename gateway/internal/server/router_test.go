package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gwconfig "github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/config"
	"github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/proxy"
	"github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/status"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/correlation"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/logger"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/problem"
)

// upstream simula un servicio interno y registra lo que recibe.
type upstream struct {
	srv   *httptest.Server
	mu    sync.Mutex
	paths []string
	corr  []string
	delay time.Duration
	state string // ready | degraded | not_ready
}

func newUpstream(t *testing.T, name string) *upstream {
	t.Helper()
	u := &upstream{state: "ready"}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.mu.Lock()
		state, delay := u.state, u.delay
		if r.URL.Path != "/health/ready" {
			u.paths = append(u.paths, r.URL.Path)
			u.corr = append(u.corr, r.Header.Get(correlation.Header))
		}
		u.mu.Unlock()

		if r.URL.Path == "/health/ready" {
			w.Header().Set("Content-Type", "application/json")
			code, redis := http.StatusOK, "up"
			switch state {
			case "not_ready":
				code = http.StatusServiceUnavailable
			case "degraded":
				redis = "down"
			}
			w.WriteHeader(code)
			_, _ = io.WriteString(w, `{"status":"`+state+`","checks":{"postgres":{"status":"up","critical":true,"latency_ms":1},`+
				`"redis":{"status":"`+redis+`","critical":false,"latency_ms":1}}}`)
			return
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		_, _ = io.WriteString(w, name)
	}))
	t.Cleanup(u.srv.Close)
	return u
}

type fixture struct {
	router    *gin.Engine
	upstreams map[string]*upstream
}

func newFixture(t *testing.T, env map[string]string) *fixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	f := &fixture{upstreams: map[string]*upstream{}}
	if env == nil {
		env = map[string]string{}
	}
	for name, key := range map[string]string{
		gwconfig.Members:      "GATEWAY_MEMBERS_URL",
		gwconfig.Booking:      "GATEWAY_BOOKING_URL",
		gwconfig.BookingIndex: "GATEWAY_BOOKING_INDEXER_URL",
		gwconfig.Benefits:     "GATEWAY_BENEFITS_URL",
		gwconfig.Training:     "GATEWAY_TRAINING_URL",
		gwconfig.Notification: "GATEWAY_NOTIFICATION_URL",
	} {
		if _, set := env[key]; set {
			continue
		}
		u := newUpstream(t, name)
		f.upstreams[name] = u
		env[key] = u.srv.URL
	}
	cfg, err := gwconfig.LoadFromMap(env)
	require.NoError(t, err)

	log := logger.NewWithWriter(&bytes.Buffer{}, "api-gateway", "error")
	transport := correlation.Transport{Base: http.DefaultTransport}
	p, err := proxy.New(cfg, transport, log)
	require.NoError(t, err)

	var targets []status.Target
	for _, s := range cfg.Services {
		targets = append(targets, status.Target{Name: s.Name, BaseURL: s.URL.String()})
	}
	checker := status.NewChecker(&http.Client{Transport: transport}, 500*time.Millisecond, targets...)
	f.router = NewRouter(log, []string{"http://localhost:5173"}, p, checker)
	return f
}

func (f *fixture) do(method, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func TestRouting_CadaPrefijoVaASuServicio(t *testing.T) {
	f := newFixture(t, nil)
	cases := map[string]string{
		"/api/v1/auth/login":            gwconfig.Members,
		"/api/v1/users/42":              gwconfig.Members,
		"/api/v1/memberships":           gwconfig.Members,
		"/api/v1/classes?fecha=hoy":     gwconfig.Booking,
		"/api/v1/bookings":              gwconfig.Booking,
		"/api/v1/benefits":              gwconfig.Benefits,
		"/api/v1/training/plans":        gwconfig.Training,
		"/partner-api/v1/benefits":      gwconfig.Benefits,
		"/api/v1/nutrition/plans/7":     gwconfig.Training,
		"/api/v1/check-ins":             gwconfig.Booking,
		"/api/v1/activities":            gwconfig.Booking,
		"/api/v1/users/../memberships/": gwconfig.Members,
	}
	for path, want := range cases {
		rec := f.do(http.MethodGet, path, nil)
		assert.Equal(t, http.StatusOK, rec.Code, path)
		assert.Equal(t, want, rec.Body.String(), path)
	}
}

func TestRouting_ReenviaLaRutaCompletaYElCorrelationID(t *testing.T) {
	f := newFixture(t, nil)
	rec := f.do(http.MethodGet, "/api/v1/bookings/123", map[string]string{correlation.Header: "corr-12345678"})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "corr-12345678", rec.Header().Get(correlation.Header))
	b := f.upstreams[gwconfig.Booking]
	assert.Equal(t, []string{"/api/v1/bookings/123"}, b.paths)
	assert.Equal(t, []string{"corr-12345678"}, b.corr)
}

func TestRouting_GeneraCorrelationIDSiNoViene(t *testing.T) {
	f := newFixture(t, nil)
	rec := f.do(http.MethodGet, "/api/v1/users", nil)
	id := rec.Header().Get(correlation.Header)
	require.True(t, correlation.Valid(id))
	assert.Equal(t, []string{id}, f.upstreams[gwconfig.Members].corr)
}

func TestRouting_InternalNoSeExpone(t *testing.T) {
	f := newFixture(t, nil)
	for _, path := range []string{
		"/internal/v1/alumnos/1/vigencia",
		"/api/v1/users/internal/v1/x",
		"/api/v1/users/../../internal/v1/asignaciones",
		"/api/v1/bookings/%2e%2e/%2e%2e/internal/v1/x",
		"/API/v1/users/Internal/x",
	} {
		rec := f.do(http.MethodGet, path, nil)
		assert.Equal(t, http.StatusNotFound, rec.Code, path)
		assert.Equal(t, problem.ContentType, rec.Header().Get("Content-Type"), path)
	}
	for _, u := range f.upstreams {
		assert.Empty(t, u.paths, "ninguna solicitud /internal debe llegar a un servicio")
	}
}

func TestRouting_RutaDesconocida404(t *testing.T) {
	f := newFixture(t, nil)
	for _, path := range []string{"/", "/api/v1", "/api/v1/usersx", "/otra/cosa"} {
		rec := f.do(http.MethodGet, path, nil)
		assert.Equal(t, http.StatusNotFound, rec.Code, path)
		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), path)
		assert.Equal(t, problem.CodeNotFound, body["code"], path)
	}
}

func TestRouting_TimeoutPorRuta504(t *testing.T) {
	f := newFixture(t, map[string]string{"GATEWAY_TIMEOUT_CLASSES": "50ms"})
	b := f.upstreams[gwconfig.Booking]
	b.mu.Lock()
	b.delay = 300 * time.Millisecond
	b.mu.Unlock()

	start := time.Now()
	rec := f.do(http.MethodGet, "/api/v1/classes", nil)

	assert.Less(t, time.Since(start), 250*time.Millisecond, "debe cortar en el timeout de la ruta")
	assert.Equal(t, http.StatusGatewayTimeout, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, problem.CodeTimeout, body["code"])
}

func TestRouting_ServicioCaido502(t *testing.T) {
	f := newFixture(t, map[string]string{"GATEWAY_TRAINING_URL": "http://127.0.0.1:1"})
	rec := f.do(http.MethodGet, "/api/v1/training/plans", nil)
	assert.Equal(t, http.StatusBadGateway, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, problem.CodeDependencyUnavailable, body["code"])
}

func TestStatus_TodosReady(t *testing.T) {
	f := newFixture(t, nil)
	rec := f.do(http.MethodGet, "/api/v1/status", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var s status.Summary
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &s))
	assert.Equal(t, status.Ready, s.Status)
	require.Len(t, s.Services, 6)
	for _, svc := range s.Services {
		assert.Equal(t, status.Ready, svc.Status, svc.Name)
		assert.Equal(t, "up", svc.Checks["postgres"], svc.Name)
	}
}

func (u *upstream) setState(state string) {
	u.mu.Lock()
	u.state = state
	u.mu.Unlock()
}

func statusByService(t *testing.T, rec *httptest.ResponseRecorder) (status.Summary, map[string]string) {
	t.Helper()
	var s status.Summary
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &s))
	got := map[string]string{}
	for _, svc := range s.Services {
		got[svc.Name] = svc.Status
	}
	return s, got
}

// ADR-007: una dependencia parcial caída deja el sistema degraded con 200.
func TestStatus_DegradadoResponde200(t *testing.T) {
	f := newFixture(t, nil)
	f.upstreams[gwconfig.Booking].setState("degraded")

	rec := f.do(http.MethodGet, "/api/v1/status", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	s, got := statusByService(t, rec)
	assert.Equal(t, status.Degraded, s.Status)
	assert.Equal(t, status.Degraded, got[gwconfig.Booking])
	assert.Equal(t, status.Ready, got[gwconfig.Members])
	for _, svc := range s.Services {
		if svc.Name == gwconfig.Booking {
			assert.Equal(t, "down", svc.Checks["redis"])
		}
	}
}

// ADR-007: un servicio not_ready o unreachable deja el sistema not_ready con 503,
// aunque otro esté solo degradado.
func TestStatus_NoDisponibleResponde503(t *testing.T) {
	f := newFixture(t, map[string]string{"GATEWAY_NOTIFICATION_URL": "http://127.0.0.1:1"})
	f.upstreams[gwconfig.Benefits].setState("not_ready")
	f.upstreams[gwconfig.Booking].setState("degraded")

	rec := f.do(http.MethodGet, "/api/v1/status", nil)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)

	s, got := statusByService(t, rec)
	assert.Equal(t, status.NotReady, s.Status)
	assert.Equal(t, status.NotReady, got[gwconfig.Benefits])
	assert.Equal(t, status.Unreachable, got[gwconfig.Notification])
	assert.Equal(t, status.Degraded, got[gwconfig.Booking])
	assert.Equal(t, status.Ready, got[gwconfig.Members])
	assert.False(t, strings.Contains(rec.Body.String(), "127.0.0.1"), "no expone detalles internos")
}

func TestCORS_PreflightDelFrontend(t *testing.T) {
	f := newFixture(t, nil)
	rec := f.do(http.MethodOptions, "/api/v1/status", map[string]string{
		"Origin":                         "http://localhost:5173",
		"Access-Control-Request-Method":  "GET",
		"Access-Control-Request-Headers": correlation.Header,
	})
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "http://localhost:5173", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Headers"), correlation.Header)
}

func TestCORS_OrigenNoPermitido(t *testing.T) {
	f := newFixture(t, nil)
	rec := f.do(http.MethodGet, "/api/v1/status", map[string]string{"Origin": "http://evil.example"})
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestHealth_DelGateway(t *testing.T) {
	f := newFixture(t, nil)
	assert.Equal(t, http.StatusOK, f.do(http.MethodGet, "/health/live", nil).Code)
	assert.Equal(t, http.StatusOK, f.do(http.MethodGet, "/health/ready", nil).Code)
}
