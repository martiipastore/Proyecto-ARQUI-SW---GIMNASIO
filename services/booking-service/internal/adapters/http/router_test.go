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
)

func get(t *testing.T, path string, pingErr error) (int, health.Report) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := NewRouter(logger.NewWithWriter(&bytes.Buffer{}, "booking-service", "info"), time.Second,
		health.Check{Name: "postgres", Fn: func(context.Context) error { return pingErr }})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(nethttp.MethodGet, path, nil))
	var report health.Report
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &report))
	return rec.Code, report
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

func TestLive_NoDependeDePostgres(t *testing.T) {
	code, _ := get(t, "/health/live", errors.New("connection refused"))
	assert.Equal(t, nethttp.StatusOK, code)
}
