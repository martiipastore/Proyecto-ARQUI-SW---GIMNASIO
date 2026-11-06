package proxy

import (
	"net/http"
	"strings"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/access"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/problem"
)

func publicAuth(method, path string) bool {
	return method == http.MethodPost && (path == "/api/v1/auth/login" || path == "/api/v1/auth/activate" || path == "/api/v1/auth/resend-activation")
}

func (p *Proxy) authenticate(w http.ResponseWriter, r *http.Request, path string) bool {
	w.Header().Set("Cache-Control", "no-store")
	if len(r.Header.Values("Authorization")) != 1 || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		problem.WriteHTTP(w, r, problem.New(401, "NO_AUTENTICADO", "No autorizado", "Ingresá para acceder a tu cuenta."))
		return false
	}
	claims, err := p.verifier.Verify(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), time.Now().UTC())
	if err != nil {
		problem.WriteHTTP(w, r, problem.New(401, "NO_AUTENTICADO", "No autorizado", "Credencial inválida o vencida."))
		return false
	}
	// Solo están implementados cuenta propia/saldo y logout. Los otros recursos
	// privados quedan restringidos a administración hasta definir su matriz por
	// operación al implementar cada dominio. Nunca dar permisos por defecto.
	self := (r.Method == http.MethodGet && (path == "/api/v1/users/me" || path == "/api/v1/benefits/me" || path == "/api/v1/memberships/me" || path == "/api/v1/memberships/types")) || (r.Method == http.MethodPost && path == "/api/v1/auth/logout")
	if !self && claims.Role != access.Admin {
		problem.WriteHTTP(w, r, problem.New(403, "SIN_PERMISO", "Sin permiso", "Tu rol no permite esta operación."))
		return false
	}
	r.Header.Set("X-User-Id", claims.Subject)
	r.Header.Set("X-User-Role", claims.Role)
	r.Header.Del("Authorization")
	// Defensa adicional de las rutas implementadas; nunca se acepta del cliente.
	if len(p.serviceKey) >= 32 {
		r.Header.Set("X-Internal-Service-Key", p.serviceKey)
	}
	return true
}
