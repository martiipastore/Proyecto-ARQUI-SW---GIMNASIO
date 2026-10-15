package httpx

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/correlation"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/logger"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/problem"
)

func newTestRouter(buf *bytes.Buffer) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := NewRouter(logger.NewWithWriter(buf, "test", "debug"))
	r.GET("/panic", func(*gin.Context) { panic("algo salió mal: secreto=123") })
	r.GET("/ok", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	return r
}

func TestNewRouter_404EnFormatoProblem(t *testing.T) {
	r := newTestRouter(&bytes.Buffer{})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/no-existe", nil))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, problem.ContentType, rec.Header().Get("Content-Type"))
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, problem.CodeNotFound, body["code"])
	assert.Equal(t, rec.Header().Get(correlation.Header), body["correlation_id"])
}

func TestNewRouter_405EnFormatoProblem(t *testing.T) {
	r := newTestRouter(&bytes.Buffer{})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/ok", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, problem.ContentType, rec.Header().Get("Content-Type"))
}

func TestRecovery_500SinExponerElPanico(t *testing.T) {
	var logs bytes.Buffer
	r := newTestRouter(&logs)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.NotContains(t, rec.Body.String(), "secreto")
	assert.Contains(t, logs.String(), "pánico recuperado")
}

func TestAccessLog_IncluyeCorrelationID(t *testing.T) {
	var logs bytes.Buffer
	r := newTestRouter(&logs)
	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	req.Header.Set(correlation.Header, "corr-12345678")
	r.ServeHTTP(httptest.NewRecorder(), req)

	var line map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &line))
	assert.Equal(t, "corr-12345678", line["correlation_id"])
	assert.Equal(t, "/ok", line["path"])
	assert.EqualValues(t, 204, line["status"])
	assert.Equal(t, slog.LevelInfo.String(), line["level"])
}
