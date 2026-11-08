package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	nethttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/health"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/logger"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
)

func get(t *testing.T, path string, pingErr error) (int, health.Report) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := NewRouter(logger.NewWithWriter(&bytes.Buffer{}, "benefits-service", "info"), time.Second,
		health.Check{Name: "postgres", Fn: func(context.Context) error { return pingErr }})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(nethttp.MethodGet, path, nil))
	var report health.Report
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &report))
	return rec.Code, report
}

type quotaAuth struct{}

func (quotaAuth) Autenticar(context.Context, string) (string, error) {
	return "", domain.ErrLimiteSolicitudes
}

func TestCuotaResponde429SinEjecutarConsulta(t *testing.T) {
	r := gin.New()
	RegisterPartners(r, nil, quotaAuth{})
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			req := httptest.NewRequest(nethttp.MethodGet, "/"+version+"/accounts/usuario/balance", nil)
			req.Header.Set("X-API-Key", "secreto-no-publicar")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, 429, w.Code)
			require.Equal(t, "60", w.Header().Get("Retry-After"))
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			require.Contains(t, w.Body.String(), "RATE_LIMITED")
			require.NotContains(t, w.Body.String(), "secreto-no-publicar")
		})
	}
}

func TestReady_ConPostgresDisponible(t *testing.T) {
	code, report := get(t, "/health/ready", nil)
	assert.Equal(t, nethttp.StatusOK, code)
	assert.Equal(t, health.StatusReady, report.Status)
}

func TestReady_ConPostgresCaido(t *testing.T) {
	code, report := get(t, "/health/ready", errors.New("connection refused"))
	assert.Equal(t, nethttp.StatusServiceUnavailable, code)
	assert.Equal(t, health.StatusDown, report.Checks["postgres"].Status)
}
