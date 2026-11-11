// Package config carga la configuración de benefits-service desde el entorno.
package config

import (
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/config"
)

// Config de benefits-service.
type Config struct {
	MessagingEnabled            bool
	AMQPURL                     string
	ExternalRegistrationEnabled bool
	ServiceKey, MembersURL      string
	HTTPAddr                    string
	DBDSN                       string
	LogLevel                    string
	ShutdownTimeout             time.Duration
	HealthTimeout               time.Duration
	// Etapa 13 (D-92): acreditación en MELT por cada asistencia.
	MeltEnabled  bool
	MeltURL      string
	MeltKey      string
	MeltTimeout  time.Duration
	MeltInterval time.Duration
}

// Load lee la configuración del entorno.
func Load() (Config, error) {
	l := config.New()
	cfg := Config{
		MessagingEnabled:            l.String("BENEFITS_MESSAGING_ENABLED", "false") == "true",
		AMQPURL:                     l.String("BENEFITS_AMQP_URL", ""),
		ExternalRegistrationEnabled: l.String("BENEFITS_EXTERNAL_REGISTRATION_ENABLED", "false") == "true",
		ServiceKey:                  l.String("GYM_INTERNAL_SERVICE_KEY", ""),
		MembersURL:                  l.String("BENEFITS_MEMBERS_URL", "http://members-service:8081"),
		HTTPAddr:                    l.String("BENEFITS_HTTP_ADDR", ":8083"),
		DBDSN:                       l.Required("BENEFITS_DB_DSN"),
		LogLevel:                    l.String("LOG_LEVEL", "info"),
		ShutdownTimeout:             l.Duration("SHUTDOWN_TIMEOUT", 10*time.Second),
		HealthTimeout:               l.Duration("HEALTH_CHECK_TIMEOUT", 2*time.Second),
		MeltEnabled:                 l.String("BENEFITS_MELT_ENABLED", "false") == "true",
		MeltURL:                     l.String("BENEFITS_MELT_URL", "https://melt-gateway-prueba.onrender.com/api/v1"),
		MeltKey:                     l.String("BENEFITS_MELT_PARTNER_KEY", ""),
		// Render gratuito tarda hasta ~30 s en despertar (medido: 25 s).
		MeltTimeout:  l.Duration("BENEFITS_MELT_TIMEOUT", 40*time.Second),
		MeltInterval: l.Duration("BENEFITS_MELT_INTERVAL", 5*time.Second),
	}
	return cfg, l.Err()
}
