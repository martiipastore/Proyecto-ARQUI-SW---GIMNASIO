package http

import (
	"bytes"
	"context"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/logger"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/application"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type registrationStub struct {
	received domain.CompletarRegistro
	calls    int
}

func (s *registrationStub) Registrar(context.Context, domain.RegistroExterno) (domain.ReciboRegistro, error) {
	return domain.ReciboRegistro{}, nil
}
func (s *registrationStub) ConsultarRegistro(context.Context, string, string) (domain.ReciboRegistro, error) {
	return domain.ReciboRegistro{}, nil
}
func (s *registrationStub) ConsultarCuenta(context.Context, string) (application.Balance, error) {
	return application.Balance{}, nil
}
func (s *registrationStub) CompletarRegistro(_ context.Context, in domain.CompletarRegistro) (int64, error) {
	s.received = in
	s.calls++
	return 150, nil
}

func TestCallbackInternoValidaCredencialYContrato(t *testing.T) {
	s := &registrationStub{}
	secret := strings.Repeat("s", 32)
	router := NewRouter(logger.NewWithWriter(&bytes.Buffer{}, "benefits", "error"), time.Second)
	RegisterRegistrations(router, s, nil, secret)
	do := func(key, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/internal/v1/external-registrations/fulfill", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Internal-Service-Key", key)
		out := httptest.NewRecorder()
		router.ServeHTTP(out, r)
		return out
	}
	body := `{"registration_id":"registro","event_id":"evento-001","partner_id":"partner","user_id":"usuario","external_user_id":"externo","points":150,"reason":"Participación","reference":"evento-001"}`
	require.Equal(t, 401, do("inválida", body).Code)
	require.Zero(t, s.calls)
	require.Equal(t, 200, do(secret, body).Code)
	require.Equal(t, "evento-001", s.received.EventoID)
	require.Equal(t, "usuario", s.received.UsuarioID)
	require.EqualValues(t, 150, s.received.Puntos)
	require.Equal(t, 400, do(secret, `{"points":150,"campo_desconocido":true}`).Code)
	require.Equal(t, 1, s.calls)
}
