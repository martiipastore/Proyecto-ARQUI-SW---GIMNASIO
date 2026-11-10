// Package config carga la configuración de notification-worker desde el entorno.
package config

import (
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/config"
)

// Config de notification-worker.
type Config struct {
	MessagingEnabled                                     bool
	AMQPURL                                              string
	SMTPHost, SMTPPort, SMTPFrom, SMTPUser, SMTPPassword string
	SMTPAllowPlaintext                                   bool
	HTTPAddr                                             string
	MongoURI                                             string
	MongoDatabase                                        string
	LogLevel                                             string
	ShutdownTimeout                                      time.Duration
	HealthTimeout                                        time.Duration
}

// Load lee la configuración del entorno.
func Load() (Config, error) {
	l := config.New()
	cfg := Config{
		MessagingEnabled:   l.String("NOTIFICATION_MESSAGING_ENABLED", "false") == "true",
		AMQPURL:            l.String("NOTIFICATION_AMQP_URL", ""),
		SMTPHost:           l.String("NOTIFICATION_SMTP_HOST", "mailpit"),
		SMTPPort:           l.String("NOTIFICATION_SMTP_PORT", "1025"),
		SMTPFrom:           l.String("NOTIFICATION_SMTP_FROM", "gimnasio@gym.local"),
		SMTPUser:           l.String("NOTIFICATION_SMTP_USER", ""),
		SMTPPassword:       l.String("NOTIFICATION_SMTP_PASSWORD", ""),
		SMTPAllowPlaintext: l.String("NOTIFICATION_SMTP_ALLOW_PLAINTEXT", "false") == "true",
		HTTPAddr:           l.String("NOTIFICATION_HTTP_ADDR", ":8085"),
		MongoURI:           l.Required("NOTIFICATION_MONGO_URI"),
		MongoDatabase:      l.String("NOTIFICATION_MONGO_DB", "notifications_db"),
		LogLevel:           l.String("LOG_LEVEL", "info"),
		ShutdownTimeout:    l.Duration("SHUTDOWN_TIMEOUT", 10*time.Second),
		HealthTimeout:      l.Duration("HEALTH_CHECK_TIMEOUT", 2*time.Second),
	}
	return cfg, l.Err()
}
