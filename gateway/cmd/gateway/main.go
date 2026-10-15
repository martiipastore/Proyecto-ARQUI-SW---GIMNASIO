// Comando gateway: api-gateway, único punto de entrada del frontend y de los partners.
//
// En esta etapa: routing por prefijo hacia cada servicio, timeout por ruta,
// propagación de X-Correlation-ID, bloqueo de /internal/*, CORS para el frontend
// y GET /api/v1/status. JWT, rate limiting e inyección de identidad se agregan
// en etapas posteriores.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	gwconfig "github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/config"
	"github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/proxy"
	"github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/server"
	"github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/status"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/correlation"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/httpserver"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/logger"
)

const serviceName = "api-gateway"

func main() {
	cfg, err := gwconfig.Load()
	log := logger.New(serviceName, cfg.LogLevel)
	if err != nil {
		log.Error("configuración inválida", slog.Any("error", err))
		os.Exit(1)
	}
	if err := run(cfg, log); err != nil {
		log.Error("el gateway terminó con error", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(cfg gwconfig.Config, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	transport := correlation.Transport{Base: http.DefaultTransport}

	p, err := proxy.New(cfg, transport, log)
	if err != nil {
		return err
	}

	targets := make([]status.Target, 0, len(cfg.Services))
	for _, s := range cfg.Services {
		targets = append(targets, status.Target{Name: s.Name, BaseURL: s.URL.String()})
	}
	checker := status.NewChecker(&http.Client{Transport: transport}, cfg.StatusTimeout, targets...)

	router := server.NewRouter(log, cfg.CORSOrigins, p, checker)
	return httpserver.Run(ctx, log, httpserver.Options{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})
}
