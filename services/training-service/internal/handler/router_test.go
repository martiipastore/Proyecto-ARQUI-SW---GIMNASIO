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
	"github.com/GuadiViti/Arquitectura-de-Software/services/training-service/internal/service"
)

type fakeDB struct{ err error }

func (f fakeDB) Ping(context.Context) error { return f.err }

func ready(t *testing.T, db fakeDB) (int, health.Report) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := NewRouter(logger.NewWithWriter(&bytes.Buffer{}, "training-service", "info"), service.NewReadiness(db), time.Second)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	var report health.Report
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &report))
	return rec.Code, report
}

func TestReady_ConMongoDisponible(t *testing.T) {
	code, report := ready(t, fakeDB{})
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, health.StatusUp, report.Checks["mongodb"].Status)
}

func TestReady_ConMongoCaido(t *testing.T) {
	code, report := ready(t, fakeDB{err: errors.New("server selection timeout")})
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, health.StatusDown, report.Checks["mongodb"].Status)
}
