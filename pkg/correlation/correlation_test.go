package correlation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRouter(seen *string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware())
	r.GET("/", func(c *gin.Context) {
		*seen = FromContext(c.Request.Context())
		c.Status(http.StatusNoContent)
	})
	return r
}

func TestMiddleware_ConservaUnIDValido(t *testing.T) {
	var seen string
	r := newRouter(&seen)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(Header, "3c5e7a9b-1d2f-4a6b-8c0d-2e4f6a8b0c1d")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, "3c5e7a9b-1d2f-4a6b-8c0d-2e4f6a8b0c1d", seen)
	assert.Equal(t, "3c5e7a9b-1d2f-4a6b-8c0d-2e4f6a8b0c1d", rec.Header().Get(Header))
}

func TestMiddleware_GeneraIDSiFaltaOEsInvalido(t *testing.T) {
	for name, value := range map[string]string{
		"sin header":   "",
		"muy corto":    "abc",
		"con espacios": "hola mundo 1234",
		"inyección":    "abc\r\nX-Evil: 1",
	} {
		t.Run(name, func(t *testing.T) {
			var seen string
			r := newRouter(&seen)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if value != "" {
				req.Header[Header] = []string{value}
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			require.True(t, Valid(seen), "debe generar un ID válido, obtuvo %q", seen)
			assert.NotEqual(t, value, seen)
			assert.Equal(t, seen, rec.Header().Get(Header))
		})
	}
}

func TestNewID_EsUUIDv4(t *testing.T) {
	id := NewID()
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, id)
	assert.NotEqual(t, id, NewID())
}

func TestTransport_PropagaElIDDelContext(t *testing.T) {
	var received string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Header.Get(Header)
	}))
	defer srv.Close()

	client := &http.Client{Transport: Transport{}}
	req, err := http.NewRequestWithContext(NewContext(context.Background(), "corr-12345678"), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, "corr-12345678", received)
}
