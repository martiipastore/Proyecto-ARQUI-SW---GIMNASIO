// Package config carga la configuración de members-service desde el entorno.
package config

import (
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/config"
)

// Config de members-service.
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
		HTTPAddr:        l.String("MEMBERS_HTTP_ADDR", ":8081"),
		DBDSN:           l.Required("MEMBERS_DB_DSN"),
		LogLevel:        l.String("LOG_LEVEL", "info"),
		ShutdownTimeout: l.Duration("SHUTDOWN_TIMEOUT", 10*time.Second),
		HealthTimeout:   l.Duration("HEALTH_CHECK_TIMEOUT", 2*time.Second),
	}
	return cfg, l.Err()
}
