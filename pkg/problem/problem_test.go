package problem

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/correlation"
)

func TestNew_DerivaElTypeDelCode(t *testing.T) {
	p := New(http.StatusConflict, "SIN_CUPO", "Sin cupo", "La clase no tiene lugares.")
	assert.Equal(t, "https://gym.local/problems/sin-cupo", p.Type)
	assert.Equal(t, 409, p.Status)
}

func TestWriteHTTP_FormatoRFC7807(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings", nil)
	req = req.WithContext(correlation.NewContext(context.Background(), "corr-12345678"))
	rec := httptest.NewRecorder()

	p := New(http.StatusBadRequest, CodeValidation, "Solicitud inválida", "Hay 1 error.")
	p.Errors = []FieldError{{Field: "clase_id", Message: "obligatorio"}}
	WriteHTTP(rec, req, p)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, ContentType, rec.Header().Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "VALIDACION", body["code"])
	assert.Equal(t, "/api/v1/bookings", body["instance"])
	assert.Equal(t, "corr-12345678", body["correlation_id"])
	assert.EqualValues(t, 400, body["status"])
	assert.Len(t, body["errors"], 1)
}

func TestMarshalJSON_ExtensionesNoPisanCamposEstandar(t *testing.T) {
	p := New(http.StatusServiceUnavailable, CodeDependencyUnavailable, "No disponible", "")
	p.Extensions = map[string]any{"checks": map[string]string{"postgres": "down"}, "status": "pisado"}

	raw, err := json.Marshal(p)
	require.NoError(t, err)

	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))
	assert.EqualValues(t, 503, body["status"])
	assert.Contains(t, body, "checks")
	assert.NotContains(t, body, "detail", "los campos vacíos se omiten")
}

func TestInternal_NoExponeDetalles(t *testing.T) {
	p := Internal()
	assert.Equal(t, 500, p.Status)
	assert.Equal(t, CodeInternal, p.Code)
	assert.NotContains(t, p.Detail, "panic")
}
