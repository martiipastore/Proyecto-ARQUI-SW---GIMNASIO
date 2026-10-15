// Package handler es la capa HTTP de training-service: arma las rutas, valida el
// formato de entrada y traduce errores a RFC 7807. Solo llama a la capa service.
package handler

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/health"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/httpx"
	"github.com/GuadiViti/Arquitectura-de-Software/services/training-service/internal/service"
)

// NewRouter crea el router HTTP del servicio.
func NewRouter(log *slog.Logger, readiness *service.Readiness, healthTimeout time.Duration) *gin.Engine {
	r := httpx.NewRouter(log)
	health.Register(r, healthTimeout, readiness.Checks()...)
	return r
}
