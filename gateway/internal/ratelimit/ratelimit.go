// Package ratelimit implementa el control de tráfico del gateway (ARCHITECTURE
// §5.1 paso 7 y §5.3; etapa 12 de la entrega final): token bucket por clave en
// Redis, con un script atómico para que varias instancias del gateway compartan
// el mismo contador sin carreras.
package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"math"
	"sync/atomic"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Policy describe un balde: Rate fichas por segundo con capacidad Burst.
// FailClosed indica qué pasa si Redis no responde: rechazar (login) o dejar pasar.
type Policy struct {
	Name       string
	Rate       float64
	Burst      int
	FailClosed bool
}

// Decision es el resultado de consumir una ficha.
type Decision struct {
	Allowed    bool
	RetryAfter time.Duration
	// Unavailable indica que no se pudo consultar el contador y la política falla cerrada.
	Unavailable bool
}

// Store consume una ficha del balde identificado por key.
type Store interface {
	Take(ctx context.Context, key string, p Policy) (Decision, error)
}

// tokenBucket recarga las fichas según el tiempo transcurrido, consume una si
// hay y devuelve {permitido, milisegundos de espera}. Usa TIME de Redis para que
// el reloj sea el mismo para todas las instancias del gateway.
var tokenBucket = goredis.NewScript(`
local rate = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
local data = redis.call('HMGET', KEYS[1], 'tokens', 'ts')
local tokens = tonumber(data[1])
local ts = tonumber(data[2])
if tokens == nil or ts == nil then
  tokens = burst
  ts = now
end
tokens = math.min(burst, tokens + math.max(0, now - ts) * rate)
local allowed = 0
local wait = 0
if tokens >= 1 then
  tokens = tokens - 1
  allowed = 1
else
  wait = math.ceil((1 - tokens) / rate)
end
redis.call('HSET', KEYS[1], 'tokens', tostring(tokens), 'ts', tostring(now))
redis.call('PEXPIRE', KEYS[1], math.ceil(burst / rate) + 1000)
return {allowed, wait}
`)

// Redis guarda los baldes con el prefijo gw:rl: (ARCHITECTURE §8, prefijos por servicio).
type Redis struct {
	Client  *goredis.Client
	Timeout time.Duration
}

// Take consume una ficha de forma atómica.
func (s *Redis) Take(ctx context.Context, key string, p Policy) (Decision, error) {
	if p.Rate <= 0 || p.Burst < 1 {
		return Decision{}, errors.New("política de rate limiting inválida")
	}
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	// Fichas por milisegundo, como espera el script.
	res, err := tokenBucket.Run(ctx, s.Client, []string{key}, p.Rate/1000, p.Burst).Int64Slice()
	if err != nil {
		return Decision{}, err
	}
	if len(res) != 2 {
		return Decision{}, errors.New("respuesta inesperada del script de rate limiting")
	}
	return Decision{Allowed: res[0] == 1, RetryAfter: time.Duration(res[1]) * time.Millisecond}, nil
}

// Ping verifica la conexión; Redis es una dependencia parcial del gateway (ADR-007).
func (s *Redis) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	return s.Client.Ping(ctx).Err()
}

// Metrics cuenta decisiones por política y errores de Redis (§5.3).
type Metrics struct {
	Allowed     map[string]*atomic.Int64
	Rejected    map[string]*atomic.Int64
	RedisErrors atomic.Int64
}

// Limiter aplica las políticas sobre un Store.
type Limiter struct {
	store   Store
	log     *slog.Logger
	Metrics *Metrics
}

// New crea el limitador para las políticas indicadas.
func New(store Store, log *slog.Logger, policies ...Policy) *Limiter {
	m := &Metrics{Allowed: map[string]*atomic.Int64{}, Rejected: map[string]*atomic.Int64{}}
	for _, p := range policies {
		m.Allowed[p.Name] = &atomic.Int64{}
		m.Rejected[p.Name] = &atomic.Int64{}
	}
	return &Limiter{store: store, log: log, Metrics: m}
}

// Allow consume una ficha para id dentro de la política. Si Redis falla, deja
// pasar salvo que la política falle cerrada (login), según ARCHITECTURE §5.3.
func (l *Limiter) Allow(ctx context.Context, p Policy, id string) Decision {
	d, err := l.store.Take(ctx, "gw:rl:"+p.Name+":"+id, p)
	if err != nil {
		l.Metrics.RedisErrors.Add(1)
		l.log.WarnContext(ctx, "rate limiting sin Redis", slog.String("policy", p.Name), slog.Bool("fail_closed", p.FailClosed))
		if p.FailClosed {
			return Decision{Allowed: false, Unavailable: true, RetryAfter: time.Second}
		}
		return Decision{Allowed: true}
	}
	if d.Allowed {
		inc(l.Metrics.Allowed, p.Name)
	} else {
		inc(l.Metrics.Rejected, p.Name)
		l.log.InfoContext(ctx, "solicitud limitada", slog.String("policy", p.Name))
	}
	return d
}

func inc(m map[string]*atomic.Int64, name string) {
	if c, ok := m[name]; ok {
		c.Add(1)
	}
}

// RetryAfterSeconds redondea hacia arriba, con mínimo 1, para el header Retry-After.
func RetryAfterSeconds(d time.Duration) int {
	s := int(math.Ceil(d.Seconds()))
	if s < 1 {
		return 1
	}
	return s
}

// HashKey identifica una API key sin guardarla ni registrarla en claro.
func HashKey(apiKey string) string {
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:])
}
