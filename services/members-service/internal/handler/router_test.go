package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/health"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/logger"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/service"
)

type fakeDB struct{ err error }

func (f fakeDB) Ping(context.Context) error { return f.err }

func ready(t *testing.T, db fakeDB) (int, health.Report) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := NewRouter(logger.NewWithWriter(&bytes.Buffer{}, "members-service", "info"), service.NewReadiness(db), time.Second)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	var report health.Report
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &report))
	return rec.Code, report
}

func TestReady_ConPostgresDisponible(t *testing.T) {
	code, report := ready(t, fakeDB{})
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, health.StatusUp, report.Checks["postgres"].Status)
}

func TestReady_ConPostgresCaido(t *testing.T) {
	code, report := ready(t, fakeDB{err: errors.New("connection refused")})
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, health.StatusNotReady, report.Status)
	assert.Equal(t, health.StatusDown, report.Checks["postgres"].Status)
}

func TestLive_SiempreOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := NewRouter(logger.NewWithWriter(&bytes.Buffer{}, "members-service", "info"),
		service.NewReadiness(fakeDB{err: errors.New("caída")}), time.Second)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}
