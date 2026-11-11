// Comando indexer de booking-service (booking-indexer): mantiene el modelo de
// lectura de clases en OpenSearch consumiendo clase.actualizada (CQRS de
// lectura, etapa 8, ADR-014). Con -reindex reconstruye el índice desde
// booking_db y termina.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "time/tzdata" // fecha y hora locales del documento en America/Argentina/Buenos_Aires

	"github.com/gin-gonic/gin"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/health"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/httpserver"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/logger"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/messaging"
	indexeramqp "github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/adapters/amqp"
	bookinghttp "github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/adapters/http"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/adapters/opensearch"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/adapters/postgres"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/application"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/config"
)

const serviceName = "booking-indexer"

type relojSistema struct{}

func (relojSistema) Ahora() time.Time { return time.Now() }

func main() {
	reindex := flag.Bool("reindex", false, "reconstruir el índice de clases desde booking_db y terminar")
	flag.Parse()
	cfg, err := config.Load()
	log := logger.New(serviceName, cfg.LogLevel)
	if err != nil {
		log.Error("configuración inválida", slog.Any("error", err))
		os.Exit(1)
	}
	if *reindex {
		err = reindexar(cfg, log)
	} else {
		err = run(cfg, log)
	}
	if err != nil {
		log.Error("el indexer terminó con error", slog.Any("error", err))
		os.Exit(1)
	}
}

func nombreIndice() string { return fmt.Sprintf("clases-%d", time.Now().UnixMilli()) }

func reindexar(cfg config.Config, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := postgres.Open(ctx, cfg.DBDSN)
	if err != nil {
		return err
	}
	defer db.Close()
	client := &opensearch.Client{URL: cfg.OpenSearchURL, HTTP: &http.Client{Timeout: 30 * time.Second}}
	inicio := time.Now()
	out, err := application.Reindexar(ctx, db, client, nombreIndice())
	if err != nil {
		return err
	}
	log.Info("índice reconstruido", slog.String("indice", out.Indice), slog.String("anterior", out.Anterior), slog.Int64("clases", out.Clases), slog.Duration("duracion", time.Since(inicio)))
	return nil
}

func run(cfg config.Config, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := postgres.Open(ctx, cfg.DBDSN)
	if err != nil {
		return err
	}
	defer db.Close()

	// ADR-007: PostgreSQL es parcial (solo lo usa el reindexado completo).
	checks := []health.Check{{Name: "postgres", Fn: db.Ping, Partial: true}}
	var consumer *messaging.Consumer
	metricas := &indexeramqp.Metricas{}
	if cfg.IndexerEnabled {
		if cfg.AMQPURL == "" {
			return errors.New("BOOKING_INDEXER_ENABLED requiere BOOKING_AMQP_URL")
		}
		client := &opensearch.Client{URL: cfg.OpenSearchURL, HTTP: &http.Client{Timeout: 5 * time.Second}}
		// Sin OpenSearch no hay dónde indexar: es dependencia crítica del indexer.
		checks = append(checks, health.Check{Name: "opensearch", Fn: client.Ping})
		adapter := indexeramqp.Adapter{App: application.NewIndexador(client, relojSistema{}), Metricas: metricas, Espera: 12 * time.Second}
		consumer = &messaging.Consumer{URL: cfg.AMQPURL, Route: messaging.Route{Queue: "booking-indexer.clase-actualizada", Key: "clase.actualizada"}, Handle: adapter.Handle, Permanent: indexeramqp.Permanent, Log: log}
		checks = append(checks, health.Check{Name: consumer.Route.Queue, Fn: consumer.Ready, Partial: true})
		consumerCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		defer func() { cancel(); <-done }()
		// Se consume recién con el alias creado: escribir antes crearía un índice
		// automático con tipos adivinados en lugar del mapping estricto.
		go func() {
			defer close(done)
			if asegurarIndice(consumerCtx, log, client) {
				consumer.Run(consumerCtx)
			}
		}()
		log.Info("indexado de clases habilitado")
	}
	router := bookinghttp.NewRouter(log, cfg.HealthTimeout, checks...)
	router.GET("/metrics", func(c *gin.Context) {
		ultimo, maximo, promedio, cantidad := metricas.Retraso()
		var procesados, reintentos, dlq int64
		if consumer != nil {
			procesados, reintentos, dlq = consumer.Processed.Load(), consumer.Retried.Load(), consumer.Dead.Load()
		}
		body := fmt.Sprintf("clase_events_processed_total %d\nclase_events_retries_total %d\nclase_events_dlq_total %d\nclase_index_applied_total %d\nclase_index_stale_total %d\nclase_index_lag_seconds_last %.6f\nclase_index_lag_seconds_max %.6f\nclase_index_lag_seconds_avg %.6f\nclase_index_lag_samples_total %d\n",
			procesados, reintentos, dlq, metricas.Aplicados.Load(), metricas.Descartados.Load(), ultimo.Seconds(), maximo.Seconds(), promedio.Seconds(), cantidad)
		c.Data(200, "text/plain; version=0.0.4", []byte(body))
	})
	return httpserver.Run(ctx, log, httpserver.Options{
		Addr:            cfg.IndexerHTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})
}

// asegurarIndice crea el primer índice y su alias cuando OpenSearch responde.
func asegurarIndice(ctx context.Context, log *slog.Logger, client *opensearch.Client) bool {
	for ctx.Err() == nil {
		work, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := client.Asegurar(work, nombreIndice())
		cancel()
		if err == nil {
			return true
		}
		log.Warn("OpenSearch todavía no disponible; se reintentará")
		select {
		case <-ctx.Done():
		case <-time.After(2 * time.Second):
		}
	}
	return false
}
