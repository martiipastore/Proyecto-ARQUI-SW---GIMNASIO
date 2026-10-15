package health

import (
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
)

func ok(context.Context) error { return nil }

func failing(context.Context) error { return errors.New("connection refused") }

func slow(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestEvaluate_TodasLasDependenciasArriba(t *testing.T) {
	r := Evaluate(context.Background(), time.Second, Check{Name: "postgres", Fn: ok}, Check{Name: "cache", Fn: ok})
	assert.Equal(t, StatusReady, r.Status)
	assert.Equal(t, StatusUp, r.Checks["postgres"].Status)
	assert.Equal(t, StatusUp, r.Checks["cache"].Status)
}

func TestEvaluate_UnaDependenciaCaidaDejaNotReady(t *testing.T) {
	r := Evaluate(context.Background(), time.Second, Check{Name: "postgres", Fn: failing}, Check{Name: "cache", Fn: ok})
	assert.Equal(t, StatusNotReady, r.Status)
	assert.Equal(t, StatusDown, r.Checks["postgres"].Status)
	assert.Equal(t, "connection refused", r.Checks["postgres"].Error)
	assert.Equal(t, StatusUp, r.Checks["cache"].Status)
}

func TestEvaluate_ParcialCaidaDejaDegraded(t *testing.T) {
	r := Evaluate(context.Background(), time.Second,
		Check{Name: "postgres", Fn: ok},
		Check{Name: "redis", Fn: failing, Partial: true})
	assert.Equal(t, StatusDegraded, r.Status)
	assert.True(t, r.Checks["postgres"].Critical)
	assert.False(t, r.Checks["redis"].Critical)
	assert.Equal(t, StatusDown, r.Checks["redis"].Status)
}

func TestEvaluate_CriticaCaidaPrevaleceSobreParcial(t *testing.T) {
	r := Evaluate(context.Background(), time.Second,
		Check{Name: "postgres", Fn: failing},
		Check{Name: "redis", Fn: failing, Partial: true})
	assert.Equal(t, StatusNotReady, r.Status)
}

func TestEvaluate_RespetaElTimeout(t *testing.T) {
	start := time.Now()
	r := Evaluate(context.Background(), 50*time.Millisecond, Check{Name: "mongo", Fn: slow})
	assert.Less(t, time.Since(start), time.Second)
	assert.Equal(t, StatusNotReady, r.Status)
	assert.Equal(t, "timeout", r.Checks["mongo"].Error)
}

func TestEvaluate_SinChecksEstaReady(t *testing.T) {
	assert.Equal(t, StatusReady, Evaluate(context.Background(), time.Second).Status)
}

func serve(t *testing.T, path string, checks ...Check) (*httptest.ResponseRecorder, Report) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Register(r, time.Second, checks...)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var report Report
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &report))
	return rec, report
}

func TestRegister_Live(t *testing.T) {
	rec, report := serve(t, "/health/live", Check{Name: "postgres", Fn: failing})
	assert.Equal(t, http.StatusOK, rec.Code, "live no depende de las dependencias")
	assert.Equal(t, "alive", report.Status)
}

func TestRegister_ReadyOK(t *testing.T) {
	rec, report := serve(t, "/health/ready", Check{Name: "postgres", Fn: ok})
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, StatusReady, report.Status)
}

func TestRegister_Degraded200(t *testing.T) {
	rec, report := serve(t, "/health/ready", Check{Name: "postgres", Fn: ok}, Check{Name: "redis", Fn: failing, Partial: true})
	assert.Equal(t, http.StatusOK, rec.Code, "una dependencia parcial caída no retira el proceso")
	assert.Equal(t, StatusDegraded, report.Status)
}

func TestRegister_Ready503ConDetalle(t *testing.T) {
	rec, report := serve(t, "/health/ready", Check{Name: "postgres", Fn: failing})
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, StatusNotReady, report.Status)
	assert.Equal(t, StatusDown, report.Checks["postgres"].Status)
	assert.NotEmpty(t, report.Checks["postgres"].Error)
}
