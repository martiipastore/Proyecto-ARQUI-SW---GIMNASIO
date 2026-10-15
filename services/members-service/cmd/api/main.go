// Comando api de members-service: usuarios, login, roles y membresías (patrón Capas).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/httpserver"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/logger"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/config"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/handler"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/repository"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/service"
)

const serviceName = "members-service"

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

	db, err := repository.NewPostgres(ctx, cfg.DBDSN)
	if err != nil {
		return err
	}
	defer db.Close()

	router := handler.NewRouter(log, service.NewReadiness(db), cfg.HealthTimeout)
	return httpserver.Run(ctx, log, httpserver.Options{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})
}
