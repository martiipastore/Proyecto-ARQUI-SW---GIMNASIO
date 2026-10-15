// Package logger crea el logger estructurado (slog en JSON) de los servicios.
//
// Cada línea incluye el nombre del servicio y, si el context lo tiene, el
// correlation_id. Usá siempre las variantes con context (InfoContext,
// ErrorContext…) para que el correlation_id aparezca. Nunca loguees emails,
// DNI, contraseñas, tokens ni API keys.
package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/correlation"
)

// New crea un logger JSON a stdout con el atributo service.
func New(service, level string) *slog.Logger {
	return NewWithWriter(os.Stdout, service, level)
}

// NewWithWriter es como New pero escribe en w (útil en tests).
func NewWithWriter(w io.Writer, service, level string) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: ParseLevel(level)})
	return slog.New(contextHandler{Handler: h}).With(slog.String("service", service))
}

// ParseLevel convierte "debug", "info", "warn" o "error" en un nivel de slog (por defecto info).
func ParseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// contextHandler agrega el correlation_id del context a cada registro.
type contextHandler struct {
	slog.Handler
}

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := correlation.FromContext(ctx); id != "" {
		r.AddAttrs(slog.String("correlation_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{Handler: h.Handler.WithGroup(name)}
}
