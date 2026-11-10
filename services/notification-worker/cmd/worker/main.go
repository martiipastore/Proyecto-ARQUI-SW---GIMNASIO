// Comando worker de notification-worker: consumirá membresia.activada y enviará
// el email de confirmación con estado durable, reintentos y DLQ (RN-05).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/health"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/httpserver"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/httpx"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/logger"
	"github.com/GuadiViti/Arquitectura-de-Software/services/notification-worker/internal/config"
	"github.com/GuadiViti/Arquitectura-de-Software/services/notification-worker/internal/consumer"
	"github.com/GuadiViti/Arquitectura-de-Software/services/notification-worker/internal/mailer"
	"github.com/GuadiViti/Arquitectura-de-Software/services/notification-worker/internal/store"
	"github.com/gin-gonic/gin"
)

const serviceName = "notification-worker"

func main() {
	cfg, err := config.Load()
	log := logger.New(serviceName, cfg.LogLevel)
	if err != nil {
		log.Error("configuración inválida", slog.Any("error", err))
		os.Exit(1)
	}
	if err := run(cfg, log); err != nil {
		log.Error("el worker terminó con error", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(cfg config.Config, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.NewMongo(cfg.MongoURI, cfg.MongoDatabase)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := db.Close(closeCtx); err != nil {
			log.Warn("error al cerrar MongoDB", slog.Any("error", err))
		}
	}()

	router := httpx.NewRouter(log)
	checks := []health.Check{{Name: "mongodb", Fn: db.Ping}}
	if cfg.MessagingEnabled {
		if cfg.AMQPURL == "" {
			return fmt.Errorf("NOTIFICATION_AMQP_URL requerida para mensajería")
		}
		worker := &consumer.Worker{URL: cfg.AMQPURL, Log: log, Failures: db, Processor: consumer.Processor{Store: db, Mailer: mailer.SMTP{Host: cfg.SMTPHost, Port: cfg.SMTPPort, From: cfg.SMTPFrom, Username: cfg.SMTPUser, Password: cfg.SMTPPassword, AllowPlaintext: cfg.SMTPAllowPlaintext}}}
		workerCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		defer func() { cancel(); <-done }()
		go func() { defer close(done); worker.Run(workerCtx) }()
		checks = append(checks, health.Check{Name: "rabbitmq", Fn: worker.Ready})
		router.GET("/metrics", func(c *gin.Context) {
			c.Data(200, "text/plain; version=0.0.4", []byte(fmt.Sprintf("notification_processed_total %d\nnotification_retries_total %d\nnotification_dlq_total %d\n", worker.Processed.Load(), worker.Retries.Load(), worker.Failed.Load())))
		})
	}
	health.Register(router, cfg.HealthTimeout, checks...)

	return httpserver.Run(ctx, log, httpserver.Options{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})
}
