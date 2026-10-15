// Package config carga la configuración de booking-service (api e indexer) desde el entorno.
package config

import (
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/config"
)

// Config de booking-service.
type Config struct {
	HTTPAddr        string
	IndexerHTTPAddr string
	DBDSN           string
	LogLevel        string
	ShutdownTimeout time.Duration
	HealthTimeout   time.Duration
}

// Load lee la configuración del entorno.
func Load() (Config, error) {
	l := config.New()
	cfg := Config{
		HTTPAddr:        l.String("BOOKING_HTTP_ADDR", ":8082"),
		IndexerHTTPAddr: l.String("BOOKING_INDEXER_HTTP_ADDR", ":8086"),
		DBDSN:           l.Required("BOOKING_DB_DSN"),
		LogLevel:        l.String("LOG_LEVEL", "info"),
		ShutdownTimeout: l.Duration("SHUTDOWN_TIMEOUT", 10*time.Second),
		HealthTimeout:   l.Duration("HEALTH_CHECK_TIMEOUT", 2*time.Second),
	}
	return cfg, l.Err()
}
