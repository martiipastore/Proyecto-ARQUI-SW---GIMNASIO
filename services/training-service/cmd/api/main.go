// Comando api de training-service: planes de entrenamiento, planes alimenticios,
// mediciones y consultas (patrón Capas, MongoDB). Acá solo se cablea.
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "time/tzdata" // "hoy" en America/Argentina/Buenos_Aires aunque la imagen no la traiga

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/correlation"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/httpserver"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/logger"
	"github.com/GuadiViti/Arquitectura-de-Software/services/training-service/internal/config"
	"github.com/GuadiViti/Arquitectura-de-Software/services/training-service/internal/handler"
	"github.com/GuadiViti/Arquitectura-de-Software/services/training-service/internal/repository"
	"github.com/GuadiViti/Arquitectura-de-Software/services/training-service/internal/service"
)

const serviceName = "training-service"

type reloj struct{}

func (reloj) Now() time.Time { return time.Now() }

type ids struct{}

func (ids) NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

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
	if cfg.Enabled {
		if len(cfg.ServiceKey) < 32 {
			return errors.New("TRAINING_ENABLED requiere GYM_INTERNAL_SERVICE_KEY de al menos 32 caracteres")
		}
		schemaCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		err := db.EnsureSchema(schemaCtx)
		cancel()
		if err != nil {
			return fmt.Errorf("preparar colecciones de training_db: %w", err)
		}
		zona, err := time.LoadLocation("America/Argentina/Buenos_Aires")
		if err != nil {
			return err
		}
		members := repository.Members{URL: cfg.MembersURL, Secret: cfg.ServiceKey, HTTP: &http.Client{Timeout: cfg.MembersTimeout, Transport: correlation.Transport{}}}
		handler.Register(router, service.NewTraining(db, members, reloj{}, ids{}, zona), cfg.ServiceKey, log)
		log.Info("entrenamiento y nutrición habilitados")
	}
	return httpserver.Run(ctx, log, httpserver.Options{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})
}
