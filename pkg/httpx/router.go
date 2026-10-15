// Package httpx arma el router Gin base que comparten todos los servicios:
// correlation ID, log de acceso, recuperación de pánicos y respuestas 404/405
// en formato RFC 7807.
package httpx

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/correlation"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/problem"
)

// NewRouter crea un *gin.Engine con los middlewares comunes.
func NewRouter(log *slog.Logger) *gin.Engine {
	r := gin.New()
	r.HandleMethodNotAllowed = true
	r.Use(correlation.Middleware(), AccessLog(log), Recovery(log))
	r.NoRoute(func(c *gin.Context) {
		problem.Write(c, problem.NotFound("No existe el recurso "+c.Request.URL.Path+"."))
	})
	r.NoMethod(func(c *gin.Context) {
		problem.Write(c, problem.New(http.StatusMethodNotAllowed, problem.CodeMethodNotAllowed,
			"Método no permitido", "El método "+c.Request.Method+" no está permitido en "+c.Request.URL.Path+"."))
	})
	return r
}

// AccessLog registra cada solicitud. Los endpoints /health/* se registran en nivel debug
// para no llenar los logs con los healthchecks.
func AccessLog(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		level := slog.LevelInfo
		switch {
		case strings.HasPrefix(c.Request.URL.Path, "/health/"):
			level = slog.LevelDebug
		case c.Writer.Status() >= http.StatusInternalServerError:
			level = slog.LevelError
		case c.Writer.Status() >= http.StatusBadRequest:
			level = slog.LevelWarn
		}
		log.Log(c.Request.Context(), level, "solicitud HTTP",
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", c.Writer.Status()),
			slog.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
	}
}

// Recovery convierte un pánico en un 500 RFC 7807 sin exponer detalles al cliente.
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				log.ErrorContext(c.Request.Context(), "pánico recuperado", slog.Any("panic", rec),
					slog.String("path", c.Request.URL.Path))
				problem.Write(c, problem.Internal())
			}
		}()
		c.Next()
	}
}
