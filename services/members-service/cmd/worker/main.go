// Comando worker procesa registros externos, correos y vencimientos de membresías.
// Se ejecuta como proceso persistente separado cuando la API corre en Vercel.
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
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/logger"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/config"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/repository"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/service"
)

func main() {
	cfg, err := config.Load()
	log := logger.New("members-worker", cfg.LogLevel)
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
	if !cfg.IdentityEnabled {
		return fmt.Errorf("MEMBERS_IDENTITY_ENABLED debe ser true")
	}
	if len(cfg.ServiceKey) < 32 {
		return fmt.Errorf("GYM_INTERNAL_SERVICE_KEY debe tener al menos 32 caracteres")
	}
	if cfg.WorkerInterval <= 0 {
		return fmt.Errorf("MEMBERS_WORKER_INTERVAL debe ser mayor que cero")
	}
	signer, err := access.NewSigner(cfg.JWTPrivateKey)
	if err != nil {
		return fmt.Errorf("MEMBERS_JWT_PRIVATE_KEY inválida: requiere clave privada RSA PEM de al menos 2048 bits")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := repository.NewPostgres(ctx, cfg.DBDSN)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return err
	}
	identity, err := service.NewIdentity(db, repository.BenefitsHTTP{
		URL: cfg.BenefitsURL, Secret: cfg.ServiceKey, Client: &http.Client{Timeout: 4 * time.Second},
	}, repository.SMTP{
		Host: cfg.SMTPHost, Port: cfg.SMTPPort, From: cfg.SMTPFrom,
		Username: cfg.SMTPUser, Password: cfg.SMTPPassword,
		AllowPlaintext: cfg.SMTPAllowPlaintext,
	}, cfg.ActivationKey, cfg.PublicURL, signer)
	if err != nil {
		return err
	}
	memberships := service.NewMemberships(db, nil)
	ticker := time.NewTicker(cfg.WorkerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			workCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			if _, err := memberships.Expire(workCtx); err != nil {
				log.Warn("vencimiento de membresías pendiente; se reintentará", slog.Any("error", err))
			}
			if err := identity.Process(workCtx); err != nil {
				log.Warn("registro o correo pendiente; se reintentará", slog.Any("error", err))
			}
			cancel()
		}
	}
}
