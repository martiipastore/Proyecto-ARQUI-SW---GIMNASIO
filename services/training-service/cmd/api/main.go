// Comando api de training-service: planes de entrenamiento, planes alimenticios,
// mediciones y consultas (patrón Capas, MongoDB).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/httpserver"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/logger"
	"github.com/GuadiViti/Arquitectura-de-Software/services/training-service/internal/config"
	"github.com/GuadiViti/Arquitectura-de-Software/services/training-service/internal/handler"
	"github.com/GuadiViti/Arquitectura-de-Software/services/training-service/internal/repository"
	"github.com/GuadiViti/Arquitectura-de-Software/services/training-service/internal/service"
)

const serviceName = "training-service"

func main() {
	cfg, err := config.Load()
	log := logger.New(serviceName, cfg.LogLevel)
	if err != nil {
		log.Error("configuración inválida", slog.Any("error", err))
		os.Exit(1)
	}
	if err := run(cfg, log); err != nil {
		log.Error("el servicio terminó con error", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(cfg config.Config, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := repository.NewMongo(cfg.MongoURI, cfg.MongoDatabase)
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

	router := handler.NewRouter(log, service.NewReadiness(db), cfg.HealthTimeout)
	return httpserver.Run(ctx, log, httpserver.Options{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})
}
