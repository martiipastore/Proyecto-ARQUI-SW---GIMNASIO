// Package http implementa los adaptadores de entrada HTTP de Benefits.
// La API de partners recibe /v1; el gateway publica /partner-api/v1.
// NewRouter registra salud; RegisterPartners conecta los casos de uso.
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
