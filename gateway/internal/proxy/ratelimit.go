package proxy

import (
	"net"
	"net/http"
	"strconv"

	"github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/ratelimit"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/problem"
)

// Limits son las tres políticas de ARCHITECTURE §5.2: login por IP (falla
// cerrado), usuarios autenticados por sub y partners por API key (fallan abierto).
type Limits struct {
	Limiter *ratelimit.Limiter
	Login   ratelimit.Policy
	User    ratelimit.Policy
	Partner ratelimit.Policy
}

// WithRateLimit activa el control de tráfico (etapa 12).
func (p *Proxy) WithRateLimit(l Limits) *Proxy {
	p.limits = &l
	return p
}

// allow aplica la política que corresponde a la ruta y responde 429 (o 503 si
// el login no puede verificarse) cuando se excede. Devuelve false si ya respondió.
func (p *Proxy) allow(w http.ResponseWriter, r *http.Request, cleaned string, partner bool) bool {
	if p.limits == nil {
		return true
	}
	var policy ratelimit.Policy
	var id string
	switch {
	case partner:
		policy = p.limits.Partner
		if key := r.Header.Get("X-API-Key"); key != "" {
			id = "key:" + ratelimit.HashKey(key)
		} else {
			id = "ip:" + clientIP(r)
		}
	case publicAuth(r.Method, cleaned):
		policy = p.limits.Login
		id = "ip:" + clientIP(r)
	default:
		policy = p.limits.User
		if sub := r.Header.Get("X-User-Id"); sub != "" {
			id = "user:" + sub
		} else {
			id = "ip:" + clientIP(r)
		}
	}
	d := p.limits.Limiter.Allow(r.Context(), policy, id)
	if d.Allowed {
		return true
	}
	retry := ratelimit.RetryAfterSeconds(d.RetryAfter)
	w.Header().Set("Retry-After", strconv.Itoa(retry))
	switch {
	case d.Unavailable:
		problem.WriteHTTP(w, r, problem.New(http.StatusServiceUnavailable, problem.CodeDependencyUnavailable,
			"Servicio no disponible", "No se pudo verificar el límite de intentos de ingreso. Reintentá en unos segundos."))
	case partner:
		problem.WritePartnerHTTP(w, r, problem.New(http.StatusTooManyRequests, "RATE_LIMITED",
			"Demasiadas solicitudes", "Se superó el límite de solicitudes. Reintentá después de Retry-After segundos."))
	default:
		problem.WriteHTTP(w, r, problem.New(http.StatusTooManyRequests, "LIMITE_EXCEDIDO",
			"Demasiadas solicitudes", "Hiciste demasiadas solicitudes seguidas. Reintentá en "+strconv.Itoa(retry)+" s."))
	}
	return false
}

// clientIP usa la dirección de la conexión. No se confía en X-Forwarded-For:
// el gateway es el borde y un cliente podría falsearlo para evadir el límite.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
