package server

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// D8/RN-26: el gateway adapta la ruta y elimina identidad no confiable,
// pero deja la autenticación de la API key al servicio de Benefits.
func TestPartners_RutaCabecerasYQuery(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/accounts/alumno-1/movements", r.URL.Path)
		require.Equal(t, "page=2&size=10", r.URL.RawQuery)
		for _, h := range []string{"X-Partner-Id", "X-User-Id", "X-User-Role", "X-Internal-Test"} {
			require.Empty(t, r.Header.Get(h))
		}
		require.Equal(t, "api-key-de-prueba", r.Header.Get("X-API-Key"))
		w.WriteHeader(200)
	}))
	defer upstream.Close()
	f := newFixture(t, map[string]string{"GATEWAY_BENEFITS_URL": upstream.URL})
	response := f.do("GET", "/partner-api/v1/accounts/alumno-1/movements?page=2&size=10", map[string]string{"X-API-Key": "api-key-de-prueba", "X-Partner-Id": "falso", "X-User-Id": "falso", "X-User-Role": "ADMIN", "X-Internal-Test": "falso"})
	require.Equal(t, 200, response.Code)
}

func TestPartners_Errores502Y504Contractuales(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(map[bool]string{false: "502", true: "504"}[timeout], func(t *testing.T) {
			var destino string
			var closeServer func()
			if timeout {
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					select {
					case <-r.Context().Done():
					case <-time.After(time.Second):
					}
				}))
				destino = s.URL
				closeServer = s.Close
			} else {
				s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
				destino = s.URL
				s.Close()
				closeServer = func() {}
			}
			defer closeServer()
			f := newFixture(t, map[string]string{"GATEWAY_BENEFITS_URL": destino, "GATEWAY_TIMEOUT_PARTNER_API": "20ms"})
			response := f.do("POST", "/partner-api/v1/accounts/alumno-1/credits", map[string]string{"X-Correlation-ID": "valor-no-uuid"})
			expected, code := 502, "UNAVAILABLE"
			if timeout {
				expected = 504
				code = "TIMEOUT"
			}
			require.Equal(t, expected, response.Code)
			var body map[string]any
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			require.Equal(t, code, body["code"])
			require.Contains(t, body, "correlationId")
			require.NotContains(t, body, "correlation_id")
			require.Regexp(t, `^[0-9a-f-]{36}$`, body["correlationId"])
			require.Equal(t, "application/problem+json", response.Header().Get("Content-Type"))
			if !timeout {
				require.NotEmpty(t, response.Header().Get("Retry-After"))
			}
		})
	}
}
