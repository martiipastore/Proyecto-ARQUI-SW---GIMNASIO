// Package server arma el router del api-gateway.
package server

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/middleware"
	"github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/proxy"
	"github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/status"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/health"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/httpx"
)

// NewRouter crea el router: endpoints propios del gateway (/health/*, /api/v1/status)
// y, para todo lo demás, el proxy por prefijo.
func NewRouter(log *slog.Logger, corsOrigins []string, p *proxy.Proxy, checker *status.Checker) *gin.Engine {
	r := httpx.NewRouter(log)
	r.Use(middleware.CORS(corsOrigins))

	health.Register(r, 0)
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
