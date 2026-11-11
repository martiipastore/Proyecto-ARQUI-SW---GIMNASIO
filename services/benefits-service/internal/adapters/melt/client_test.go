package melt

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
)

// Etapa 13: el adaptador anticorrupción envía la solicitud del contrato y
// traduce cada respuesta de MELT a un resultado del dominio.
func TestClienteEnviaLaSolicitudDelContrato(t *testing.T) {
	var got map[string]any
	var headers http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v1/beneficios/acreditaciones", r.URL.Path)
		headers = r.Header.Clone()
		raw, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(raw, &got))
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"solicitudId":"ref","usuarioId":"7","puntos":100,"saldoPuntos":100,"tipo":"acreditacion_externa","estado":"acreditada","creadoEn":"2026-11-11T16:00:00Z"}`)
	}))
	defer srv.Close()
	c := Client{URL: srv.URL + "/api/v1/", Key: "clave-de-prueba", HTTP: srv.Client()}
	r, err := c.Acreditar(context.Background(), " alumno@gym.local ", 100, "metalfitness-asistencia-x", "Asistencia a clase en Metal Fitness")
	require.NoError(t, err)
	require.Equal(t, domain.ResultadoAcreditado, r.Tipo)
	require.Equal(t, "clave-de-prueba", headers.Get("X-Partner-Key"))
	require.Equal(t, "application/json", headers.Get("Content-Type"))
	require.Equal(t, map[string]any{"email": "alumno@gym.local", "puntos": float64(100), "referencia": "metalfitness-asistencia-x", "descripcion": "Asistencia a clase en Metal Fitness"}, got)
}

func TestTraducirRespuestas(t *testing.T) {
	cases := []struct {
		status int
		body   string
		tipo   string
	}{
		{201, `{}`, domain.ResultadoAcreditado},
		{200, `{}`, domain.ResultadoAcreditado},
		{202, `{"solicitudId":"r","estado":"pendiente","puntos":100,"puntosAcreditados":0,"registroUrl":"https://melt-gateway-prueba.onrender.com/registro#tok","venceEn":"2026-11-18T16:00:00Z"}`, domain.ResultadoPendiente},
		{401, `{"error":{"code":"PARTNER_NO_AUTORIZADO","message":"x"}}`, domain.ResultadoReintentar},
		{400, `{"error":{"code":"DATOS_INVALIDOS","message":"x"}}`, domain.ResultadoRechazado},
		{404, `{"error":{"code":"USUARIO_NO_EXISTE","message":"x"}}`, domain.ResultadoRechazado},
		{409, `{"error":{"code":"SOLICITUD_EN_CONFLICTO","message":"x"}}`, domain.ResultadoRechazado},
		{415, ``, domain.ResultadoRechazado},
		{422, `{"error":{"code":"SALDO_FUERA_DE_RANGO","message":"x"}}`, domain.ResultadoRechazado},
	}
	for _, c := range cases {
		r, err := Traducir(c.status, []byte(c.body))
		require.NoErrorf(t, err, "status %d", c.status)
		require.Equalf(t, c.tipo, r.Tipo, "status %d", c.status)
	}
	p, _ := Traducir(202, []byte(cases[2].body))
	require.Equal(t, "https://melt-gateway-prueba.onrender.com/registro#tok", p.RegistroURL)
	require.Equal(t, time.Date(2026, 11, 18, 16, 0, 0, 0, time.UTC), p.RegistroVence.UTC())
	x, _ := Traducir(409, []byte(cases[6].body))
	require.Contains(t, x.Motivo, "SOLICITUD_EN_CONFLICTO")

	for _, status := range []int{500, 502, 503, 504} {
		_, err := Traducir(status, nil)
		require.ErrorIs(t, err, ErrIncierto, "status %d se reintenta con la misma referencia", status)
	}
	_, err := Traducir(202, []byte(`{"estado":"pendiente"}`))
	require.ErrorIs(t, err, ErrIncierto, "un 202 sin enlace no se da por resuelto")
	_, err = Traducir(202, []byte(`{"registroUrl":"http://inseguro","venceEn":"2026-11-18T16:00:00Z"}`))
	require.ErrorIs(t, err, ErrIncierto, "solo se acepta un enlace HTTPS")
}

func TestClienteTimeoutYRedSonInciertos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer srv.Close()
	c := Client{URL: srv.URL, Key: "k", HTTP: &http.Client{Timeout: 50 * time.Millisecond}}
	_, err := c.Acreditar(context.Background(), "a@b.c", 100, "ref", "")
	require.ErrorIs(t, err, ErrIncierto)

	caido := Client{URL: "http://127.0.0.1:1", Key: "k", HTTP: &http.Client{Timeout: time.Second}}
	_, err = caido.Acreditar(context.Background(), "a@b.c", 100, "ref", "")
	require.ErrorIs(t, err, ErrIncierto)
}
