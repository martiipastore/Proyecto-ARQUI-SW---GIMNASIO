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
	// Etapa 8: SearchEnabled pasa GET /classes a OpenSearch; IndexerEnabled
	// activa el consumidor de clase.actualizada del booking-indexer.
	SearchEnabled  bool
	IndexerEnabled bool
	OpenSearchURL  string
	SearchTimeout  time.Duration
	// Etapa 9: caché del catálogo de actividades en Redis (ADR-007, D-88).
	CacheEnabled  bool
	RedisAddr     string
	RedisPassword string
	CacheTTL      time.Duration
	CacheTimeout  time.Duration
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
		SearchEnabled:      l.String("BOOKING_SEARCH_ENABLED", "false") == "true",
		IndexerEnabled:     l.String("BOOKING_INDEXER_ENABLED", "false") == "true",
		OpenSearchURL:      l.String("BOOKING_OPENSEARCH_URL", "http://opensearch:9200"),
		SearchTimeout:      l.Duration("BOOKING_SEARCH_TIMEOUT", 2*time.Second),
		CacheEnabled:       l.String("BOOKING_CACHE_ENABLED", "false") == "true",
		RedisAddr:          l.String("BOOKING_REDIS_ADDR", "redis:6379"),
		RedisPassword:      l.String("BOOKING_REDIS_PASSWORD", ""),
		CacheTTL:           l.Duration("BOOKING_CACHE_TTL", 10*time.Minute),
		CacheTimeout:       l.Duration("BOOKING_CACHE_TIMEOUT", 100*time.Millisecond),
	}
	return cfg, l.Err()
}
