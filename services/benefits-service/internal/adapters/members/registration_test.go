package members

import (
	"context"
	"encoding/json"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestContratoRegistroInternoYReciboExterno(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "/internal/v1/external-registrations", r.URL.Path)
		require.Equal(t, "secreto-interno", r.Header.Get("X-Internal-Service-Key"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "partner", body["partner_id"])
		require.Equal(t, "usuario-externo", body["external_user_id"])
		require.Equal(t, "evento-001", body["event_id"])
		w.WriteHeader(202)
		_, _ = w.Write([]byte(`{"registration_id":"registro","event_id":"evento-001","external_user_id":"usuario-externo","status":"accepted"}`))
	}))
	defer server.Close()
	client := Client{URL: server.URL, Secret: "secreto-interno", HTTP: server.Client()}
	out, err := client.Registrar(context.Background(), domain.RegistroExterno{PartnerID: "partner", ExternalUserID: "usuario-externo", EventoID: "evento-001"})
	require.NoError(t, err)
	require.Equal(t, "registro", out.ID)
	require.Equal(t, "usuario-externo", out.ExternalUserID)
}
