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

// bookingAllowed es la autorización gruesa de booking.v1 para roles distintos del
// administrador; booking-service vuelve a autorizar sobre el recurso (RN-30).
// El resto de booking (catálogo, horarios, generación, capacidad, cancelación
// de clases e ingreso por DNI, D-82) queda solo para administración.
func bookingAllowed(method, path, role string) bool {
	segments := strings.Split(strings.TrimPrefix(path, "/api/v1/"), "/")
	switch {
	case method == http.MethodGet && path == "/api/v1/activities":
		return true
	case method == http.MethodGet && path == "/api/v1/classes/mine":
		return role == access.Teacher
	case method == http.MethodGet && (path == "/api/v1/classes" || (len(segments) == 2 && segments[0] == "classes" && segments[1] != "")):
		return role == access.Student || role == access.Teacher
	case path == "/api/v1/bookings":
		return method == http.MethodPost && role == access.Student
	case method == http.MethodGet && path == "/api/v1/bookings/me":
		return role == access.Student
	case method == http.MethodPost && len(segments) == 3 && segments[0] == "bookings" && segments[1] != "" && segments[2] == "cancel":
		return role == access.Student
	}
	return false
}

// benefitsAllowed es la autorización gruesa de la cuponera interna (D-94): el
// alumno ve el catálogo, canjea y consulta sus cupones. La gestión del catálogo
// y de los cupones queda para el administrador; benefits-service vuelve a
// autorizar por rol y cuenta propia.
func benefitsAllowed(method, path, role string) bool {
	switch {
	case method == http.MethodGet && (path == "/api/v1/benefits/catalog" || path == "/api/v1/benefits/me/redemptions"):
		return role == access.Student
	case method == http.MethodPost && path == "/api/v1/benefits/redemptions":
		return role == access.Student
	}
	return false
}

// trainingAllowed es la autorización gruesa de training.v1 (etapa 10): cada rol
// solo llega a sus rutas. training-service vuelve a verificar la asignación
// vigente con members en cada operación (RN-30, D-33). El administrador no
// opera planes: la regla por rol la aplica también el servicio.
func trainingAllowed(method, path, role string) bool {
	segments := strings.Split(strings.TrimPrefix(path, "/api/v1/"), "/")
	get, post := method == http.MethodGet, method == http.MethodPost
	switch {
	case get && (path == "/api/v1/training/plans/me" || path == "/api/v1/nutrition/plans/me" || path == "/api/v1/nutrition/measurements/me" || path == "/api/v1/nutrition/questions/me"):
		return role == access.Student
	case post && path == "/api/v1/nutrition/questions":
		return role == access.Student
	case (get || post) && path == "/api/v1/training/plans":
		return role == access.Teacher
	case (get || post) && (path == "/api/v1/nutrition/plans" || path == "/api/v1/nutrition/measurements"):
		return role == access.Nutritionist
	case get && path == "/api/v1/nutrition/questions":
		return role == access.Nutritionist
	case post && len(segments) == 4 && segments[0] == "nutrition" && segments[1] == "questions" && segments[2] != "" && segments[3] == "answer":
		return role == access.Nutritionist
	}
	return false
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
	self := (r.Method == http.MethodGet && (path == "/api/v1/users/me" || path == "/api/v1/benefits/me" || path == "/api/v1/benefits/me/movements" || path == "/api/v1/benefits/me/melt" || path == "/api/v1/memberships/me" || path == "/api/v1/memberships/types")) || (r.Method == http.MethodPost && path == "/api/v1/auth/logout")
	// Profesor y nutricionista ven sus alumnos asignados (users.v1, RN-30);
	// members vuelve a verificar la asignación sobre el recurso.
	professional := r.Method == http.MethodGet && path == "/api/v1/users/me/students" && (claims.Role == access.Teacher || claims.Role == access.Nutritionist)
	if !self && !professional && !bookingAllowed(r.Method, path, claims.Role) && !trainingAllowed(r.Method, path, claims.Role) && !benefitsAllowed(r.Method, path, claims.Role) && claims.Role != access.Admin {
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
