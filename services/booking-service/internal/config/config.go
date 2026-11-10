// Package config carga la configuración de booking-service (api e indexer) desde el entorno.
package config

import (
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/config"
)

// Config de booking-service.
type Config struct {
	MessagingEnabled bool
	AMQPURL          string
	HTTPAddr         string
	IndexerHTTPAddr  string
	DBDSN            string
	LogLevel         string
	ShutdownTimeout  time.Duration
	HealthTimeout    time.Duration
	// Enabled activa actividades, clases, reservas e ingreso (etapa 4). Apagado,
	// el servicio solo expone salud, como antes de la etapa.
	Enabled            bool
	MembersURL         string
	InternalServiceKey string
	MembersTimeout     time.Duration
	CloseInterval      time.Duration
}

// Load lee la configuración del entorno.
func Load() (Config, error) {
	l := config.New()
	cfg := Config{
		MessagingEnabled:   l.String("BOOKING_MESSAGING_ENABLED", "false") == "true",
		AMQPURL:            l.String("BOOKING_AMQP_URL", ""),
		HTTPAddr:           l.String("BOOKING_HTTP_ADDR", ":8082"),
		IndexerHTTPAddr:    l.String("BOOKING_INDEXER_HTTP_ADDR", ":8086"),
		DBDSN:              l.Required("BOOKING_DB_DSN"),
		LogLevel:           l.String("LOG_LEVEL", "info"),
		ShutdownTimeout:    l.Duration("SHUTDOWN_TIMEOUT", 10*time.Second),
		HealthTimeout:      l.Duration("HEALTH_CHECK_TIMEOUT", 2*time.Second),
		Enabled:            l.String("BOOKING_ENABLED", "false") == "true",
		MembersURL:         l.String("BOOKING_MEMBERS_URL", "http://members-service:8081"),
		InternalServiceKey: l.String("GYM_INTERNAL_SERVICE_KEY", ""),
		MembersTimeout:     l.Duration("BOOKING_MEMBERS_TIMEOUT", 2*time.Second),
		CloseInterval:      l.Duration("BOOKING_CLOSE_INTERVAL", 30*time.Second),
	}
	return cfg, l.Err()
}
