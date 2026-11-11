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
	"strings"
	"syscall"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/health"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/httpserver"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/logger"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/messaging"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/telemetry"
	bookingamqp "github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/amqp"
	benefitshttp "github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/http"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/melt"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/members"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/postgres"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/system"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/application"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/config"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
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

	// Etapa 13 (D-92, ADR-009 v2): cada asistencia registra en la misma
	// transacción un envío de puntos a MELT; este proceso los entrega en segundo
	// plano. Una falla del proveedor no afecta la asistencia ni los puntos locales,
	// por eso MELT no forma parte de /health/ready.
	var enviosMelt *application.EnviosMelt
	if cfg.MeltEnabled {
		if len(cfg.ServiceKey) < 32 {
			return fmt.Errorf("BENEFITS_MELT_ENABLED requiere GYM_INTERNAL_SERVICE_KEY de al menos 32 caracteres")
		}
		if cfg.MeltKey == "" {
			log.Warn("BENEFITS_MELT_PARTNER_KEY vacía: MELT responderá 401 y los envíos quedan pendientes hasta configurarla")
		}
		bookingEvents.ConEnviosMelt()
		contactos := members.Client{URL: cfg.MembersURL, Secret: cfg.ServiceKey, HTTP: &http.Client{Timeout: 3 * time.Second}}
		proveedor := melt.Client{URL: cfg.MeltURL, Key: cfg.MeltKey, HTTP: &http.Client{Timeout: cfg.MeltTimeout}}
		enviosMelt = application.NewEnviosMelt(db, proveedor, contactos, system.Clock{})
		meltCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		defer func() { cancel(); <-done }()
		go func() {
			defer close(done)
			ticker := time.NewTicker(cfg.MeltInterval)
			defer ticker.Stop()
			for {
				if n, err := enviosMelt.Procesar(meltCtx, 20); err != nil && meltCtx.Err() == nil {
					log.Warn("no se pudieron procesar los envíos a MELT", slog.Any("error", err))
				} else if n > 0 {
					log.Info("envíos a MELT procesados", slog.Int("cantidad", n))
				}
				select {
				case <-meltCtx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
		log.Info("acreditación en MELT activa", slog.Int64("puntos_por_asistencia", domain.PuntosMeltPorAsistencia))
	}
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
	// Etapa 14: Prometheus lee los eventos consumidos y los envíos a MELT.
	if cfg.MessagingEnabled || enviosMelt != nil {
		router.GET("/metrics", func(c *gin.Context) {
			var b strings.Builder
			if cfg.MessagingEnabled {
				var processed, retried, dead int64
				for _, w := range workers {
					processed += w.Processed.Load()
					retried += w.Retried.Load()
					dead += w.Dead.Load()
				}
				fmt.Fprintf(&b, "booking_events_processed_total %d\nbooking_events_retries_total %d\nbooking_events_dlq_total %d\n", processed, retried, dead)
			}
			if enviosMelt != nil {
				for _, r := range []struct {
					nombre string
					n      int64
				}{{"acreditado", enviosMelt.Acreditados.Load()}, {"pendiente_registro", enviosMelt.Pendientes.Load()}, {"rechazado", enviosMelt.Rechazados.Load()}, {"reintento", enviosMelt.Reintentos.Load()}} {
					fmt.Fprintf(&b, "melt_envios_intentos_total{resultado=%q} %d\n", r.nombre, r.n)
				}
			}
			c.Data(200, "text/plain; version=0.0.4", []byte(b.String()))
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
	if enviosMelt != nil {
		benefitshttp.RegisterMeltHistory(router, enviosMelt, cfg.ServiceKey)
	}
	// Etapa 7: administración del Club. Sin GYM_INTERNAL_SERVICE_KEY válida las
	// rutas responden 401: solo el gateway autenticado puede llegar a ellas.
	benefitshttp.RegisterAdministration(router, application.NewAdministracion(db, system.Clock{}, system.IDs{}, system.Secrets{}), cfg.ServiceKey, log)
	// D-94: cuponera interna (catálogo y canjes de la aplicación). No se publica
	// en la API de partners, que sigue sin canjes (D-76).
	benefitshttp.RegisterCuponera(router, application.NewCuponera(db, system.Clock{}, system.IDs{}, system.Codigos{}), cfg.ServiceKey, log)
	router.NoRoute(benefitshttp.PartnerFallback)
	router.NoMethod(benefitshttp.PartnerFallback)
	return httpserver.Run(ctx, log, httpserver.Options{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})
}
