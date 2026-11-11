// Package server arma el router del api-gateway.
package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/middleware"
	"github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/proxy"
	"github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/ratelimit"
	"github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/status"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/health"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/httpx"
)

// NewRouter crea el router: endpoints propios del gateway (/health/*, /api/v1/status)
// y, para todo lo demás, el proxy por prefijo. Con rate limiting activo, Redis es
// una dependencia parcial (ADR-007). /metrics (etapa 14) expone las requests por
// clase de respuesta, el estado de cada servicio y los contadores del limitador.
func NewRouter(log *slog.Logger, corsOrigins []string, p *proxy.Proxy, checker *status.Checker, limiter *ratelimit.Limiter, checks ...health.Check) *gin.Engine {
	r := httpx.NewRouter(log)
	requests := &requestMetrics{}
	r.Use(requests.middleware())
	r.Use(middleware.CORS(corsOrigins))

	health.Register(r, 2*time.Second, checks...)
	r.GET("/metrics", func(c *gin.Context) {
		var b strings.Builder
		b.WriteString(requests.text())
		b.WriteString(servicesText(checker.Check(c.Request.Context())))
		if limiter != nil {
			b.WriteString(metricsText(limiter.Metrics))
		}
		c.Data(http.StatusOK, "text/plain; version=0.0.4", []byte(b.String()))
	})
	r.GET("/api/v1/status", func(c *gin.Context) {
		summary := checker.Check(c.Request.Context())
		code := http.StatusOK
		if summary.Status == status.NotReady {
			code = http.StatusServiceUnavailable
		}
		c.JSON(code, summary)
	})

	r.NoRoute(p.Handler())
	return r
}

// metricsText escribe los contadores en formato de texto de Prometheus.
func metricsText(m *ratelimit.Metrics) string {
	names := make([]string, 0, len(m.Allowed))
	for name := range m.Allowed {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("# TYPE gateway_ratelimit_allowed_total counter\n")
	for _, n := range names {
		fmt.Fprintf(&b, "gateway_ratelimit_allowed_total{policy=%q} %d\n", n, m.Allowed[n].Load())
	}
	b.WriteString("# TYPE gateway_ratelimit_rejected_total counter\n")
	for _, n := range names {
		fmt.Fprintf(&b, "gateway_ratelimit_rejected_total{policy=%q} %d\n", n, m.Rejected[n].Load())
	}
	b.WriteString("# TYPE ratelimit_redis_errors_total counter\n")
	fmt.Fprintf(&b, "ratelimit_redis_errors_total %d\n", m.RedisErrors.Load())
	return b.String()
}
