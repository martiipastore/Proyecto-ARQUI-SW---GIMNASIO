// Comando indexer de booking-service (booking-indexer): mantendrá el modelo de
// lectura de clases en OpenSearch consumiendo clase.actualizada (CQRS de lectura).
//
// En esta etapa es solo el esqueleto: arranca, expone /health/live y
// /health/ready (verifica booking_db como dependencia parcial, que usará para el
// reindexado completo) y se apaga ordenadamente. El consumidor y OpenSearch se
// agregan más adelante.
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
	bookinghttp "github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/adapters/http"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/adapters/postgres"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/config"
)

const serviceName = "booking-indexer"

func main() {
	cfg, err := config.Load()
	log := logger.New(serviceName, cfg.LogLevel)
	if err != nil {
		log.Error("configuración inválida", slog.Any("error", err))
		os.Exit(1)
	}
	if err := run(cfg, log); err != nil {
		log.Error("el indexer terminó con error", slog.Any("error", err))
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

	// ADR-007: para el indexer, PostgreSQL es parcial (solo lo usa el reindexado completo).
	// Sus dependencias críticas (RabbitMQ y OpenSearch) se agregan con el consumidor.
	router := bookinghttp.NewRouter(log, cfg.HealthTimeout, health.Check{Name: "postgres", Fn: db.Ping, Partial: true})
	return httpserver.Run(ctx, log, httpserver.Options{
		Addr:            cfg.IndexerHTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})
}
