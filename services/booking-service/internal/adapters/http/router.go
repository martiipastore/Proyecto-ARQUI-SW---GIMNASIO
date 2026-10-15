// Package http es el adaptador de entrada HTTP (Gin) de booking-service. Traduce
// solicitudes a llamadas a los puertos de entrada de internal/application y sus
// errores a RFC 7807. No contiene reglas de negocio.
package http

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/health"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/httpx"
)

// NewRouter crea el router HTTP. checks son las dependencias propias que verifica /health/ready.
func NewRouter(log *slog.Logger, healthTimeout time.Duration, checks ...health.Check) *gin.Engine {
	r := httpx.NewRouter(log)
	health.Register(r, healthTimeout, checks...)
	return r
}
