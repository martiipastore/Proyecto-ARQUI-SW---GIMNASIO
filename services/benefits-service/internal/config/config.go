// Package config carga la configuración de benefits-service desde el entorno.
package config

import (
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/config"
)

// Config de benefits-service.
type Config struct {
	HTTPAddr        string
	DBDSN           string
	LogLevel        string
	ShutdownTimeout time.Duration
	HealthTimeout   time.Duration
}

// Load lee la configuración del entorno.
func Load() (Config, error) {
	l := config.New()
	cfg := Config{
		HTTPAddr:        l.String("BENEFITS_HTTP_ADDR", ":8083"),
		DBDSN:           l.Required("BENEFITS_DB_DSN"),
		LogLevel:        l.String("LOG_LEVEL", "info"),
		ShutdownTimeout: l.Duration("SHUTDOWN_TIMEOUT", 10*time.Second),
		HealthTimeout:   l.Duration("HEALTH_CHECK_TIMEOUT", 2*time.Second),
	}
	return cfg, l.Err()
}
