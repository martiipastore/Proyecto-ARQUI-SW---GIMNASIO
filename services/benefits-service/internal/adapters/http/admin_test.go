package http

import (
	"context"
	"encoding/json"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/application"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
)

type adminStub struct {
	application.AdministracionAPI
	actor application.Actor
	cfg   domain.ConfiguracionIntegracion
	err   error
}

func (s *adminStub) CrearIntegracion(_ context.Context, a application.Actor, c domain.ConfiguracionIntegracion) (application.IntegracionEmitida, error) {
	s.actor, s.cfg = a, c
	return application.IntegracionEmitida{Integracion: domain.Integracion{ID: "p", Config: c}, APIKey: "key-una-vez"}, s.err
}
func (s *adminStub) ActualizarIntegracion(_ context.Context, a application.Actor, _ string, c domain.ConfiguracionIntegracion) (domain.Integracion, error) {
	s.actor, s.cfg = a, c
	return domain.Integracion{ID: "p", Config: c}, s.err
}
func (s *adminStub) Revertir(_ context.Context, a application.Actor, _, _, _ string) (domain.ResultadoReversion, bool, error) {
	s.actor = a
	return domain.ResultadoReversion{}, false, s.err
}

const claveInterna = "clave-interna-de-servicio-con-32-caracteres"
const usuarioAdmin = "0b6a1f2e-9c8d-4e7f-8a6b-5c4d3e2f1a0b"

func adminRequest(t *testing.T, stub *adminStub, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterAdministration(r, stub, claveInterna, nil)
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

func gateway(role string) map[string]string {
	return map[string]string{"X-Internal-Service-Key": claveInterna, "X-User-Id": usuarioAdmin, "X-User-Role": role}
}

// ADR-006 / regla 10: sin clave interna del gateway no hay identidad confiable.
func TestAdministracion_RequiereIdentidadDelGateway(t *testing.T) {
	stub := &adminStub{}
	for _, headers := range []map[string]string{{}, {"X-User-Id": usuarioAdmin, "X-User-Role": "ADMINISTRADOR"}, {"X-Internal-Service-Key": "otra", "X-User-Id": usuarioAdmin, "X-User-Role": "ADMINISTRADOR"}} {
		w := adminRequest(t, stub, nethttp.MethodPost, "/api/v1/benefits/partners", `{"nombre":"Estudiario"}`, headers)
		require.Equal(t, 401, w.Code)
		require.Contains(t, w.Body.String(), "NO_AUTENTICADO")
	}
	require.Empty(t, stub.actor.ID)
}

// La autorización fina vive en application; el adaptador traduce a 403.
func TestAdministracion_RolNoAdministradorRecibe403(t *testing.T) {
	stub := &adminStub{err: domain.ErrSinPermiso}
	w := adminRequest(t, stub, nethttp.MethodPost, "/api/v1/benefits/partners", `{"nombre":"Estudiario"}`, gateway("ALUMNO"))
	require.Equal(t, 403, w.Code)
	require.Contains(t, w.Body.String(), "SIN_PERMISO")
	require.Equal(t, "ALUMNO", stub.actor.Rol)
}

// CA-10.1: alta ACTIVA con valores seguros; la key aparece solo en esta respuesta.
func TestAdministracion_AltaDevuelveKeyYDefaults(t *testing.T) {
	stub := &adminStub{}
	w := adminRequest(t, stub, nethttp.MethodPost, "/api/v1/benefits/partners", `{"nombre":"Estudiario","max_puntos_dia":5000}`, gateway("ADMINISTRADOR"))
	require.Equal(t, 201, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "key-una-vez", body["api_key"])
	require.Equal(t, "ACTIVO", body["estado"])
	require.True(t, stub.cfg.RequiereEvento)
	require.False(t, stub.cfg.PermiteRegistro, "registro externo se concede explícitamente")
	require.EqualValues(t, 5000, *stub.cfg.MaxPuntosDia)

	w = adminRequest(t, stub, nethttp.MethodPost, "/api/v1/benefits/partners", `{"nombre":"Estudiario","activo":false}`, gateway("ADMINISTRADOR"))
	require.Equal(t, 400, w.Code)
	w = adminRequest(t, stub, nethttp.MethodPost, "/api/v1/benefits/partners", `{"nombre":"Estudiario","api_key":"elegida"}`, gateway("ADMINISTRADOR"))
	require.Equal(t, 400, w.Code, "la credencial nunca la elige el cliente")
}

// PUT reemplaza la configuración completa; la desactivación aplica CA-10.2.
func TestAdministracion_ActualizarExigeConfiguracionCompleta(t *testing.T) {
	stub := &adminStub{}
	w := adminRequest(t, stub, nethttp.MethodPut, "/api/v1/benefits/partners/p", `{"nombre":"Estudiario","activo":false}`, gateway("ADMINISTRADOR"))
	require.Equal(t, 400, w.Code)
	full := `{"nombre":"Estudiario","activo":false,"permite_consulta":true,"permite_puntos":false,"permite_registro":false,"requiere_evento":true,"max_puntos_operacion":100,"max_puntos_dia":null,"max_solicitudes_minuto":30}`
	w = adminRequest(t, stub, nethttp.MethodPut, "/api/v1/benefits/partners/p", full, gateway("ADMINISTRADOR"))
	require.Equal(t, 200, w.Code)
	require.False(t, stub.cfg.Activo)
	require.False(t, stub.cfg.PermitePuntos)
	require.Nil(t, stub.cfg.MaxPuntosDia)
	require.Contains(t, w.Body.String(), `"estado":"INACTIVO"`)
}

// HU-11: Idempotency-Key obligatoria; errores de negocio con el catálogo §12.1.
func TestAdministracion_ReversionErrores(t *testing.T) {
	path := "/api/v1/benefits/movements/6f1c2a8e-3b7d-4c1e-9a2b-1d2e3f4a5b6c/revert"
	stub := &adminStub{}
	w := adminRequest(t, stub, nethttp.MethodPost, path, `{"motivo":"Carga por error"}`, gateway("ADMINISTRADOR"))
	require.Equal(t, 400, w.Code)
	for err, code := range map[error]string{domain.ErrSaldoInsuficiente: "SALDO_INSUFICIENTE", domain.ErrMovimientoRevertido: "TRANSICION_INVALIDA", domain.ErrConflictoIdempotencia: "CONFLICTO_IDEMPOTENCIA", domain.ErrMovimientoNoEncontrado: "NO_ENCONTRADO"} {
		stub.err = err
		h := gateway("ADMINISTRADOR")
		h["Idempotency-Key"] = "reversion-0001"
		w = adminRequest(t, stub, nethttp.MethodPost, path, `{"motivo":"Carga por error"}`, h)
		require.Contains(t, w.Body.String(), code)
		require.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
	}
}
