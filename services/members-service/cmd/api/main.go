// Comando api de members-service: usuarios, login, roles y membresías (patrón Capas).
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

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/access"
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
	if cfg.IdentityEnabled {
		if len(cfg.ServiceKey) < 32 {
			return fmt.Errorf("GYM_INTERNAL_SERVICE_KEY debe tener al menos 32 caracteres")
		}
		signer, err := access.NewSigner(cfg.JWTPrivateKey)
		if err != nil {
			return fmt.Errorf("MEMBERS_JWT_PRIVATE_KEY inválida: requiere clave privada RSA PEM de al menos 2048 bits")
		}
		if err := db.Migrate(ctx); err != nil {
			return err
		}
		identity, err := service.NewIdentity(db, repository.BenefitsHTTP{URL: cfg.BenefitsURL, Secret: cfg.ServiceKey, Client: &http.Client{Timeout: 4 * time.Second}}, repository.SMTP{Host: cfg.SMTPHost, Port: cfg.SMTPPort, From: cfg.SMTPFrom, Username: cfg.SMTPUser, Password: cfg.SMTPPassword, AllowPlaintext: cfg.SMTPAllowPlaintext}, cfg.ActivationKey, cfg.PublicURL, signer)
		if err != nil {
			return err
		}
		handler.RegisterIdentity(router, identity, cfg.ServiceKey)
		memberships := service.NewMemberships(db, nil)
		handler.RegisterMemberships(router, memberships, cfg.ServiceKey)
		if cfg.BackgroundWorkerEnabled {
			if cfg.WorkerInterval <= 0 {
				return fmt.Errorf("MEMBERS_WORKER_INTERVAL debe ser mayor que cero")
			}
			workerCtx, workerCancel := context.WithCancel(ctx)
			workerDone := make(chan struct{})
			defer func() { workerCancel(); <-workerDone }()
			go func() {
				defer close(workerDone)
				ticker := time.NewTicker(cfg.WorkerInterval)
				defer ticker.Stop()
				for {
					select {
					case <-workerCtx.Done():
						return
					case <-ticker.C:
						workCtx, cancel := context.WithTimeout(workerCtx, 15*time.Second)
						if _, err := memberships.Expire(workCtx); err != nil {
							log.Warn("vencimiento de membresías pendiente; se reintentará")
						}
						if err := identity.Process(workCtx); err != nil {
							log.Warn("registro o correo pendiente; se reintentará")
						}
						cancel()
					}
				}
			}()
		}
	}
	return httpserver.Run(ctx, log, httpserver.Options{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})
}
