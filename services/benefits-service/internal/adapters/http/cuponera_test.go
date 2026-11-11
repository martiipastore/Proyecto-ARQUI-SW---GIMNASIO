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
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/ports"
)

// Cuponera interna (D-94): contrato HTTP de benefits.v1, HU-09 y HU-20.

type cuponeraStub struct {
	application.CuponeraAPI
	actor     application.Actor
	beneficio domain.Beneficio
	clave     string
	repetido  bool
	err       error
}

func (s *cuponeraStub) Catalogo(_ context.Context, a application.Actor) ([]domain.Beneficio, error) {
	s.actor = a
	return []domain.Beneficio{{ID: "b", Nombre: "Botella", Costo: 25, VigenteDesde: "2026-11-01", Activo: true, Canjes: 7}}, s.err
}
func (s *cuponeraStub) CrearBeneficio(_ context.Context, a application.Actor, b domain.Beneficio) (domain.Beneficio, error) {
	s.actor, s.beneficio = a, b
	b.ID = "b"
	return b, s.err
}
func (s *cuponeraStub) ActualizarBeneficio(_ context.Context, a application.Actor, _ string, b domain.Beneficio) (domain.Beneficio, error) {
	s.actor, s.beneficio = a, b
	return b, s.err
}
func (s *cuponeraStub) Canjear(_ context.Context, a application.Actor, _, clave string) (domain.ResultadoCanje, bool, error) {
	s.actor, s.clave = a, clave
	return domain.ResultadoCanje{Canje: domain.CanjeBeneficio{ID: "k", Codigo: "ABCD-EF23", Estado: domain.CanjeConfirmado, Puntos: 25, UsuarioID: "u", MovimientoID: "m"}, Saldo: 5}, s.repetido, s.err
}
func (s *cuponeraStub) ListarCanjes(_ context.Context, a application.Actor, codigo, estado string, _, _ int) (ports.PaginaCanjes, error) {
	s.actor, s.clave = a, codigo+"|"+estado
	return ports.PaginaCanjes{Items: []domain.CanjeBeneficio{}}, s.err
}

func cuponeraRequest(t *testing.T, stub *cuponeraStub, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterCuponera(r, stub, claveInterna, nil)
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

func conClave(headers map[string]string, clave string) map[string]string {
	out := map[string]string{"Idempotency-Key": clave}
	for k, v := range headers {
		out[k] = v
	}
	return out
}

func TestCuponera_RequiereIdentidadDelGateway(t *testing.T) {
	stub := &cuponeraStub{}
	w := cuponeraRequest(t, stub, nethttp.MethodGet, "/api/v1/benefits/catalog", "", map[string]string{"X-User-Id": usuarioAdmin, "X-User-Role": "ALUMNO"})
	require.Equal(t, 401, w.Code)
	require.Empty(t, stub.actor.ID)
}

// El alumno ve el catálogo sin los datos de gestión (activo, cantidad de canjes).
func TestCuponera_CatalogoDelAlumno(t *testing.T) {
	stub := &cuponeraStub{}
	w := cuponeraRequest(t, stub, nethttp.MethodGet, "/api/v1/benefits/catalog", "", gateway("ALUMNO"))
	require.Equal(t, 200, w.Code)
	require.Equal(t, application.Actor{ID: usuarioAdmin, Rol: "ALUMNO"}, stub.actor)
	var body struct{ Items []map[string]any }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Items, 1)
	require.EqualValues(t, 25, body.Items[0]["costo"])
	require.NotContains(t, body.Items[0], "canjes")
	require.Nil(t, body.Items[0]["vigente_hasta"])
}

// CA-20.4 / regla 9: canjear exige Idempotency-Key; el reintento responde 200.
func TestCuponera_CanjearIdempotente(t *testing.T) {
	stub := &cuponeraStub{}
	body := `{"beneficio_id":"0b6a1f2e-9c8d-4e7f-8a6b-5c4d3e2f1a0c"}`
	w := cuponeraRequest(t, stub, nethttp.MethodPost, "/api/v1/benefits/redemptions", body, gateway("ALUMNO"))
	require.Equal(t, 400, w.Code, "sin Idempotency-Key")

	w = cuponeraRequest(t, stub, nethttp.MethodPost, "/api/v1/benefits/redemptions", body, conClave(gateway("ALUMNO"), "canje-clave-0001"))
	require.Equal(t, 201, w.Code)
	require.Equal(t, "false", w.Header().Get("Idempotent-Replayed"))
	require.Equal(t, "canje-clave-0001", stub.clave)
	require.Contains(t, w.Body.String(), `"codigo":"ABCD-EF23"`)
	require.Contains(t, w.Body.String(), `"saldo":5`)
	require.NotContains(t, w.Body.String(), "movimiento_id", "el alumno no ve datos internos")

	stub.repetido = true
	w = cuponeraRequest(t, stub, nethttp.MethodPost, "/api/v1/benefits/redemptions", body, conClave(gateway("ALUMNO"), "canje-clave-0001"))
	require.Equal(t, 200, w.Code)
	require.Equal(t, "true", w.Header().Get("Idempotent-Replayed"))

	w = cuponeraRequest(t, stub, nethttp.MethodPost, "/api/v1/benefits/redemptions", `{"beneficio_id":"x","extra":1}`, conClave(gateway("ALUMNO"), "canje-clave-0002"))
	require.Equal(t, 400, w.Code, "sin campos ajenos")
}

// Errores del catálogo de ARCHITECTURE §12.1 (RN-23, RN-24, RN-25).
func TestCuponera_MapeoDeErrores(t *testing.T) {
	casos := map[error]struct {
		status int
		code   string
	}{
		domain.ErrSaldoInsuficiente:     {422, "SALDO_INSUFICIENTE"},
		domain.ErrBeneficioNoDisponible: {422, "BENEFICIO_NO_DISPONIBLE"},
		domain.ErrBeneficioNoEncontrado: {404, "NO_ENCONTRADO"},
		domain.ErrConflictoIdempotencia: {409, "CONFLICTO_IDEMPOTENCIA"},
		domain.ErrSinPermiso:            {403, "SIN_PERMISO"},
		domain.ErrTransicionCanje:       {409, "TRANSICION_INVALIDA"},
		ports.ErrAlmacenNoDisponible:    {503, "DEPENDENCIA_NO_DISPONIBLE"},
	}
	for err, want := range casos {
		stub := &cuponeraStub{err: err}
		w := cuponeraRequest(t, stub, nethttp.MethodPost, "/api/v1/benefits/redemptions", `{"beneficio_id":"0b6a1f2e-9c8d-4e7f-8a6b-5c4d3e2f1a0c"}`, conClave(gateway("ALUMNO"), "canje-clave-0003"))
		require.Equal(t, want.status, w.Code, err.Error())
		require.Contains(t, w.Body.String(), want.code)
		require.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
	}
}

// HU-09: el alta puede omitir activo y vigente_hasta; la modificación exige activo.
func TestCuponera_AltaYModificacionDeBeneficio(t *testing.T) {
	stub := &cuponeraStub{}
	w := cuponeraRequest(t, stub, nethttp.MethodPost, "/api/v1/benefits/rewards", `{"nombre":"Botella","costo":25,"vigente_desde":"2026-11-01"}`, gateway("ADMINISTRADOR"))
	require.Equal(t, 201, w.Code)
	require.True(t, stub.beneficio.Activo)
	require.Equal(t, "", stub.beneficio.VigenteHasta)

	w = cuponeraRequest(t, stub, nethttp.MethodPut, "/api/v1/benefits/rewards/b", `{"nombre":"Botella","costo":40,"vigente_desde":"2026-11-01"}`, gateway("ADMINISTRADOR"))
	require.Equal(t, 400, w.Code, "PUT reemplaza: activo es obligatorio")
	w = cuponeraRequest(t, stub, nethttp.MethodPut, "/api/v1/benefits/rewards/b", `{"nombre":"Botella","costo":40,"vigente_desde":"2026-11-01","vigente_hasta":"2026-12-31","activo":false}`, gateway("ADMINISTRADOR"))
	require.Equal(t, 200, w.Code)
	require.False(t, stub.beneficio.Activo)
	require.EqualValues(t, 40, stub.beneficio.Costo)
	require.Equal(t, "2026-12-31", stub.beneficio.VigenteHasta)
}

func TestCuponera_BuscarCuponPorCodigo(t *testing.T) {
	stub := &cuponeraStub{}
	w := cuponeraRequest(t, stub, nethttp.MethodGet, "/api/v1/benefits/redemptions?codigo=ABCD-EF23&estado=CONFIRMADO", "", gateway("ADMINISTRADOR"))
	require.Equal(t, 200, w.Code)
	require.Equal(t, "ABCD-EF23|CONFIRMADO", stub.clave)
	w = cuponeraRequest(t, stub, nethttp.MethodGet, "/api/v1/benefits/redemptions?otro=1", "", gateway("ADMINISTRADOR"))
	require.Equal(t, 400, w.Code)
}
