// Comando api de booking-service: actividades, clases, reservas y asistencia
// (patrón Hexagonal). Acá solo se cablean adaptadores, puertos y casos de uso.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "time/tzdata" // zona America/Argentina/Buenos_Aires aunque la imagen no la traiga

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/correlation"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/health"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/httpserver"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/logger"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/messaging"
	bookinghttp "github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/adapters/http"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/adapters/members"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/adapters/opensearch"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/adapters/postgres"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/application"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/config"
)

const serviceName = "booking-service"

type relojSistema struct{}

func (relojSistema) Ahora() time.Time { return time.Now() }

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

	var relay *messaging.Relay
	checks := []health.Check{{Name: "postgres", Fn: db.Ping}}
	if cfg.MessagingEnabled {
		if !cfg.Enabled || cfg.AMQPURL == "" {
			return errors.New("mensajería requiere BOOKING_ENABLED=true y BOOKING_AMQP_URL")
		}
		// La cola del indexer se declara aunque el indexer esté apagado: los
		// clase.actualizada esperan durables y no se pierden (etapa 8).
		relay = &messaging.Relay{URL: cfg.AMQPURL, Log: log, Publish: db.PublicarOutbox, Routes: []messaging.Route{{Queue: "benefits.asistencia-registrada", Key: "asistencia.registrada"}, {Queue: "benefits.inasistencia-registrada", Key: "inasistencia.registrada"}, {Queue: "booking-indexer.clase-actualizada", Key: "clase.actualizada"}}}
		checks = append(checks, health.Check{Name: "rabbitmq", Fn: relay.Ready, Partial: true})
	}
	var indice *opensearch.Client
	if cfg.SearchEnabled {
		if !cfg.Enabled || !cfg.MessagingEnabled {
			return errors.New("la búsqueda indexada requiere BOOKING_ENABLED=true y BOOKING_MESSAGING_ENABLED=true")
		}
		indice = &opensearch.Client{URL: cfg.OpenSearchURL, HTTP: &http.Client{Timeout: cfg.SearchTimeout}}
		// ADR-014/D-45: sin OpenSearch solo falla la búsqueda; reservar sigue.
		checks = append(checks, health.Check{Name: "opensearch", Fn: indice.Ping, Partial: true})
	}
	router := bookinghttp.NewRouter(log, cfg.HealthTimeout, checks...)
	if cfg.Enabled {
		if len(cfg.InternalServiceKey) < 32 {
			return errors.New("BOOKING_ENABLED requiere GYM_INTERNAL_SERVICE_KEY de al menos 32 caracteres")
		}
		if err = db.Migrate(ctx); err != nil {
			return err
		}
		if relay != nil {
			relayCtx, cancel := context.WithCancel(ctx)
			done := make(chan struct{})
			defer func() { cancel(); <-done }()
			go func() { defer close(done); relay.Run(relayCtx) }()
		}
		reloj := relojSistema{}
		membersClient := members.Client{
			URL:    cfg.MembersURL,
			Secret: cfg.InternalServiceKey,
			HTTP:   &http.Client{Timeout: cfg.MembersTimeout, Transport: correlation.Transport{}},
		}
		servicio := application.New(db, db, membersClient, reloj)
		if indice != nil {
			servicio.ConBusqueda(indice, db)
			log.Info("búsqueda de clases en OpenSearch habilitada")
		}
		bookinghttp.Register(router, servicio, cfg.InternalServiceKey, reloj.Ahora)
		go cerrarClases(ctx, log, servicio, cfg.CloseInterval)
		log.Info("reservas habilitadas")
	}
	return httpserver.Run(ctx, log, httpserver.Options{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})
}

// cerrarClases ejecuta periódicamente el cierre automático de clases terminadas,
// que pasa a AUSENTE las reservas sin ingreso (RN-38, CA-22.8).
func cerrarClases(ctx context.Context, log *slog.Logger, s *application.Servicio, intervalo time.Duration) {
	ticker := time.NewTicker(intervalo)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			clases, ausentes, err := s.CerrarClases(ctx)
			switch {
			case err != nil:
				log.Warn("no se pudieron cerrar las clases terminadas", slog.Any("error", err))
			case clases > 0:
				log.Info("clases finalizadas", slog.Int("clases", clases), slog.Int("ausentes", ausentes))
			}
		}
	}
}
