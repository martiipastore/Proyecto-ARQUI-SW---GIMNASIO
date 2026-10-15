// Package config carga la configuración de training-service desde el entorno.
package config

import (
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/config"
)

// Config de training-service.
type Config struct {
	HTTPAddr        string
	MongoURI        string
	MongoDatabase   string
	LogLevel        string
	ShutdownTimeout time.Duration
	HealthTimeout   time.Duration
}

// Load lee la configuración del entorno.
func Load() (Config, error) {
	l := config.New()
	cfg := Config{
		HTTPAddr:        l.String("TRAINING_HTTP_ADDR", ":8084"),
		MongoURI:        l.Required("TRAINING_MONGO_URI"),
		MongoDatabase:   l.String("TRAINING_MONGO_DB", "training_db"),
		LogLevel:        l.String("LOG_LEVEL", "info"),
		ShutdownTimeout: l.Duration("SHUTDOWN_TIMEOUT", 10*time.Second),
		HealthTimeout:   l.Duration("HEALTH_CHECK_TIMEOUT", 2*time.Second),
	}
	return cfg, l.Err()
}
