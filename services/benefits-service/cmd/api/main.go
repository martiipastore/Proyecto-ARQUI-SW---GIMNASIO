// Comando api de benefits-service: Club de Beneficios y API pública de partners
// (patrón Hexagonal). Acá solo se cablean adaptadores, puertos y casos de uso.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/health"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/httpserver"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/logger"
	benefitshttp "github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/http"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/postgres"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/config"
)

const serviceName = "benefits-service"

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

	db, err := postgres.Open(ctx, cfg.DBDSN)
	if err != nil {
		return err
	}
	defer db.Close()

	router := benefitshttp.NewRouter(log, cfg.HealthTimeout, health.Check{Name: "postgres", Fn: db.Ping})
	return httpserver.Run(ctx, log, httpserver.Options{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})
}
