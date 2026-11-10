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
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/messaging"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/telemetry"
	bookingamqp "github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/amqp"
	benefitshttp "github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/http"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/members"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/postgres"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/system"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/application"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/config"
	"github.com/gin-gonic/gin"
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

	bookingEvents := application.NewBookingEvents(db, system.Clock{}, system.IDs{})
	checks := []health.Check{{Name: "postgres", Fn: db.Ping}}
	var workers []*messaging.Consumer
	if cfg.MessagingEnabled {
		if cfg.AMQPURL == "" {
			return fmt.Errorf("BENEFITS_AMQP_URL requerida para mensajería")
		}
		consumerCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		defer func() { cancel(); <-done }()
		for _, route := range []messaging.Route{{Queue: "benefits.asistencia-registrada", Key: "asistencia.registrada"}, {Queue: "benefits.inasistencia-registrada", Key: "inasistencia.registrada"}} {
			adapter := bookingamqp.Adapter{App: bookingEvents, EventType: route.Key}
			worker := &messaging.Consumer{URL: cfg.AMQPURL, Route: route, Handle: adapter.Handle, Permanent: bookingamqp.Permanent, Log: log}
			workers = append(workers, worker)
			checks = append(checks, health.Check{Name: route.Queue, Fn: worker.Ready, Partial: true})
		}
		go func() {
			defer close(done)
			finished := make(chan struct{}, len(workers))
			for _, w := range workers {
				go func(w *messaging.Consumer) { w.Run(consumerCtx); finished <- struct{}{} }(w)
			}
			for range workers {
				<-finished
			}
		}()
	}
	router := benefitshttp.NewRouter(log, cfg.HealthTimeout, checks...)
	if cfg.MessagingEnabled {
		router.GET("/metrics", func(c *gin.Context) {
			var processed, retried, dead int64
			for _, w := range workers {
				processed += w.Processed.Load()
				retried += w.Retried.Load()
				dead += w.Dead.Load()
			}
			c.Data(200, "text/plain; version=0.0.4", []byte(fmt.Sprintf("booking_events_processed_total %d\nbooking_events_retries_total %d\nbooking_events_dlq_total %d\n", processed, retried, dead)))
		})
	}
	benefits := application.New(db, system.Clock{}, system.IDs{}, benefitshttp.ResultadoEncoder{})
	auth := application.NewAuthenticator(db)
	benefitshttp.RegisterPartners(router, benefits, auth, log)
	if cfg.ExternalRegistrationEnabled {
		if len(cfg.ServiceKey) < 32 {
			return fmt.Errorf("GYM_INTERNAL_SERVICE_KEY debe tener al menos 32 caracteres")
		}
		client := members.Client{URL: cfg.MembersURL, Secret: cfg.ServiceKey, HTTP: &http.Client{Timeout: 3500 * time.Millisecond}}
		benefitshttp.RegisterRegistrations(router, application.NewRegistro(client, db, benefits), auth, cfg.ServiceKey, log)
		benefitshttp.RegisterAccountHistory(router, bookingEvents, cfg.ServiceKey)
	}
	// Etapa 7: administración del Club. Sin GYM_INTERNAL_SERVICE_KEY válida las
	// rutas responden 401: solo el gateway autenticado puede llegar a ellas.
	benefitshttp.RegisterAdministration(router, application.NewAdministracion(db, system.Clock{}, system.IDs{}, system.Secrets{}), cfg.ServiceKey, log)
	router.NoRoute(benefitshttp.PartnerFallback)
	router.NoMethod(benefitshttp.PartnerFallback)
	return httpserver.Run(ctx, log, httpserver.Options{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})
}
