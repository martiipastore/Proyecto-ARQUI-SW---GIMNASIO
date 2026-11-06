// Comando api de benefits-service: Club de Beneficios y API pública de partners
// (patrón Hexagonal). Acá solo se cablean adaptadores, puertos y casos de uso.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/health"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/httpserver"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/logger"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/telemetry"
	benefitshttp "github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/http"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/members"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/postgres"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/system"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/application"
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
	shutdownTraces, err := telemetry.Setup(ctx, serviceName, log)
	if err != nil {
		return err
	}
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := shutdownTraces(flushCtx); err != nil {
			log.Warn("no se completó el cierre de trazas")
		}
	}()

	db, err := postgres.Open(ctx, cfg.DBDSN)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return err
	}

	router := benefitshttp.NewRouter(log, cfg.HealthTimeout, health.Check{Name: "postgres", Fn: db.Ping})
	benefits := application.New(db, system.Clock{}, system.IDs{}, benefitshttp.ResultadoEncoder{})
	auth := application.NewAuthenticator(db)
	benefitshttp.RegisterPartners(router, benefits, auth, log)
	if cfg.ExternalRegistrationEnabled {
		if len(cfg.ServiceKey) < 32 {
			return fmt.Errorf("GYM_INTERNAL_SERVICE_KEY debe tener al menos 32 caracteres")
		}
		client := members.Client{URL: cfg.MembersURL, Secret: cfg.ServiceKey, HTTP: &http.Client{Timeout: 3500 * time.Millisecond}}
		benefitshttp.RegisterRegistrations(router, application.NewRegistro(client, db, benefits), auth, cfg.ServiceKey, log)
	}
	router.NoRoute(benefitshttp.PartnerFallback)
	router.NoMethod(benefitshttp.PartnerFallback)
	return httpserver.Run(ctx, log, httpserver.Options{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})
}
