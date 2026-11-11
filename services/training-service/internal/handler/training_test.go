package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/GuadiViti/Arquitectura-de-Software/services/training-service/internal/model"
	"github.com/GuadiViti/Arquitectura-de-Software/services/training-service/internal/service"
)

type sinRepo struct{ service.Repository }
type sinMembers struct{ service.Members }
type relojFijo struct{}

func (relojFijo) Now() time.Time { return time.Now() }

type idFijo struct{}

func (idFijo) NewID() (string, error) { return "id", nil }

func (sinMembers) ProfesionalesDe(context.Context, string) ([]model.Asignacion, error) {
	return []model.Asignacion{}, nil
}

const clave = "clave-interna-de-servicio-con-32-caracteres"

func llamar(t *testing.T, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Register(r, service.NewTraining(sinRepo{}, sinMembers{}, relojFijo{}, idFijo{}, time.UTC), clave, nil)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

var alumno = map[string]string{"X-Internal-Service-Key": clave, "X-User-Id": "00000000-0000-4000-8000-000000000001", "X-User-Role": "ALUMNO"}

// ADR-006 / regla 10: sin la clave interna del gateway no hay identidad confiable.
func TestTraining_RequiereGateway(t *testing.T) {
	w := llamar(t, http.MethodGet, "/api/v1/training/plans/me", "", map[string]string{"X-User-Id": "x", "X-User-Role": "ALUMNO"})
	require.Equal(t, 401, w.Code)
	require.Contains(t, w.Body.String(), "NO_AUTENTICADO")
}

// Entrada estricta y catálogo de errores (training.v1).
func TestTraining_EntradaYErrores(t *testing.T) {
	w := llamar(t, http.MethodPost, "/api/v1/nutrition/questions", `{"pregunta":"hola","extra":1}`, alumno)
	require.Equal(t, 400, w.Code, "campos desconocidos")
	w = llamar(t, http.MethodPost, "/api/v1/nutrition/questions", `{"pregunta":"¿Fruta?"}`, alumno)
	require.Equal(t, 422, w.Code)
	require.Contains(t, w.Body.String(), "SIN_NUTRICIONISTA")
	w = llamar(t, http.MethodGet, "/api/v1/training/plans/me?alumno_id=otro", "", alumno)
	require.Equal(t, 400, w.Code, "no se elige identidad por query")
	w = llamar(t, http.MethodGet, "/api/v1/training/plans", "", alumno)
	require.Equal(t, 400, w.Code, "falta alumno_id")
	profesor := map[string]string{"X-Internal-Service-Key": clave, "X-User-Id": "00000000-0000-4000-8000-000000000010", "X-User-Role": "NUTRICIONISTA"}
	w = llamar(t, http.MethodPost, "/api/v1/training/plans", `{"alumno_id":"00000000-0000-4000-8000-000000000001"}`, profesor)
	require.Equal(t, 403, w.Code)
	require.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
}
