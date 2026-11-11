// Comando gateway: api-gateway, único punto de entrada del frontend y de los partners.
//
// En esta etapa: routing por prefijo hacia cada servicio, timeout por ruta,
// propagación de X-Correlation-ID, bloqueo de /internal/*, CORS para el frontend
// y GET /api/v1/status. JWT RS256 e identidad verificada en CB-03; la matriz
// específica de recursos futuros y los límites generales siguen por completar.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	goredis "github.com/redis/go-redis/v9"

	gwconfig "github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/config"
	"github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/proxy"
	"github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/ratelimit"
	"github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/server"
	"github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/status"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/correlation"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/health"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/httpserver"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/logger"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/telemetry"
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

	transport := correlation.Transport{Base: telemetry.Transport{Base: proxy.NewTransport()}}

	p, err := proxy.New(cfg, transport, log)
	if err != nil {
		return err
	}

	// Control de tráfico (etapa 12): token bucket en Redis con los límites de
	// ARCHITECTURE §5.2. Login falla cerrado; el resto, abierto (§5.3).
	var limiter *ratelimit.Limiter
	var checks []health.Check
	if rl := cfg.RateLimit; rl.Enabled {
		client := goredis.NewClient(&goredis.Options{Addr: rl.RedisAddr, Password: rl.RedisPassword,
			DialTimeout: rl.Timeout, ReadTimeout: rl.Timeout, WriteTimeout: rl.Timeout, MaxRetries: -1})
		defer client.Close()
		store := &ratelimit.Redis{Client: client, Timeout: rl.Timeout}
		limits := proxy.Limits{
			Login:   ratelimit.Policy{Name: "login", Rate: float64(rl.LoginPerMinute) / 60, Burst: rl.LoginPerMinute, FailClosed: true},
			User:    ratelimit.Policy{Name: "user", Rate: float64(rl.UserRate), Burst: rl.UserBurst},
			Partner: ratelimit.Policy{Name: "partner", Rate: float64(rl.PartnerRate), Burst: rl.PartnerBurst},
		}
		limiter = ratelimit.New(store, log, limits.Login, limits.User, limits.Partner)
		limits.Limiter = limiter
		p.WithRateLimit(limits)
		checks = append(checks, health.Check{Name: "redis", Fn: store.Ping, Partial: true})
		log.Info("rate limiting activo", slog.Int("login_por_minuto", rl.LoginPerMinute), slog.Int("usuario_por_segundo", rl.UserRate), slog.Int("partner_por_segundo", rl.PartnerRate))
	}

	targets := make([]status.Target, 0, len(cfg.Services))
	for _, s := range cfg.Services {
		targets = append(targets, status.Target{Name: s.Name, BaseURL: s.URL.String()})
	}
	checker := status.NewChecker(&http.Client{Transport: transport}, cfg.StatusTimeout, targets...)

	router := server.NewRouter(log, cfg.CORSOrigins, p, checker, limiter, checks...)
	return httpserver.Run(ctx, log, httpserver.Options{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})
}
