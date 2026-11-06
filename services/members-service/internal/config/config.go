// Package config carga la configuración de members-service desde el entorno.
package config

import (
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/config"
)

// Config de members-service.
type Config struct {
	IdentityEnabled                                      bool
	ServiceKey                                           string
	ActivationKey                                        string
	JWTPrivateKey                                        string
	PublicURL                                            string
	BenefitsURL                                          string
	SMTPHost, SMTPPort, SMTPFrom, SMTPUser, SMTPPassword string
	SMTPAllowPlaintext                                   bool
	HTTPAddr                                             string
	DBDSN                                                string
	LogLevel                                             string
	ShutdownTimeout                                      time.Duration
	HealthTimeout                                        time.Duration
}

// Load lee la configuración del entorno.
func Load() (Config, error) {
	l := config.New()
	cfg := Config{
		IdentityEnabled:    l.String("MEMBERS_IDENTITY_ENABLED", "false") == "true",
		ServiceKey:         l.String("GYM_INTERNAL_SERVICE_KEY", ""),
		ActivationKey:      l.String("MEMBERS_ACTIVATION_ENCRYPTION_KEY", ""),
		JWTPrivateKey:      l.String("MEMBERS_JWT_PRIVATE_KEY", ""),
		PublicURL:          l.String("MEMBERS_PUBLIC_URL", "http://localhost:5173"),
		BenefitsURL:        l.String("MEMBERS_BENEFITS_URL", "http://benefits-service:8083"),
		SMTPHost:           l.String("MEMBERS_SMTP_HOST", "mailpit"),
		SMTPPort:           l.String("MEMBERS_SMTP_PORT", "1025"),
		SMTPFrom:           l.String("MEMBERS_SMTP_FROM", "gimnasio@gym.local"),
		SMTPUser:           l.String("MEMBERS_SMTP_USER", ""),
		SMTPPassword:       l.String("MEMBERS_SMTP_PASSWORD", ""),
		SMTPAllowPlaintext: l.String("MEMBERS_SMTP_ALLOW_PLAINTEXT", "false") == "true",
		HTTPAddr:           l.String("MEMBERS_HTTP_ADDR", ":8081"),
		DBDSN:              l.Required("MEMBERS_DB_DSN"),
		LogLevel:           l.String("LOG_LEVEL", "info"),
		ShutdownTimeout:    l.Duration("SHUTDOWN_TIMEOUT", 10*time.Second),
		HealthTimeout:      l.Duration("HEALTH_CHECK_TIMEOUT", 2*time.Second),
	}
	return cfg, l.Err()
}
