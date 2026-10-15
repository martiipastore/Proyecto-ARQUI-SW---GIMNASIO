// Comando worker de notification-worker: consumirá membresia.activada y enviará
// el email de confirmación. En esta etapa es solo el esqueleto: arranca, expone
// /health/live y /health/ready (verifica notifications_db) y se apaga ordenadamente.
package main

import (
	"context"
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
	"github.com/GuadiViti/Arquitectura-de-Software/services/notification-worker/internal/store"
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
	health.Register(router, cfg.HealthTimeout, health.Check{Name: "mongodb", Fn: db.Ping})

	return httpserver.Run(ctx, log, httpserver.Options{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})
}
