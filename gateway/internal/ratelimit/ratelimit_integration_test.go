// Etapa 12 contra Redis 7 real (testcontainers): el script consume la ráfaga,
// rechaza con espera, recarga con el tiempo, aísla claves, vence y es atómico
// con solicitudes concurrentes.
package ratelimit

import (
	"bytes"
	"context"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/logger"
)

func TestTokenBucketRedisReal(t *testing.T) {
	if exec.Command("docker", "info").Run() != nil {
		t.Skip("Docker no disponible")
	}
	ctx := context.Background()
	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{Started: true, ContainerRequest: testcontainers.ContainerRequest{
		Image: "redis:7-alpine", ExposedPorts: []string{"6379/tcp"}, Cmd: []string{"redis-server", "--requirepass", "fixture-redis"},
		WaitingFor: wait.ForLog("Ready to accept connections").WithStartupTimeout(time.Minute),
	}})
	t.Cleanup(func() {
		if ctr != nil {
			_ = ctr.Terminate(context.Background())
		}
	})
	require.NoError(t, err)
	addr, err := ctr.PortEndpoint(ctx, "6379/tcp", "")
	require.NoError(t, err)
	client := goredis.NewClient(&goredis.Options{Addr: addr, Password: "fixture-redis", MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	store := &Redis{Client: client, Timeout: time.Second}
	require.NoError(t, store.Ping(ctx))

	t.Run("ráfaga, rechazo con espera y recarga", func(t *testing.T) {
		p := Policy{Name: "t", Rate: 10, Burst: 3}
		for i := 0; i < 3; i++ {
			d, err := store.Take(ctx, "gw:rl:t:a", p)
			require.NoError(t, err)
			require.True(t, d.Allowed, "ficha %d de la ráfaga", i+1)
		}
		d, err := store.Take(ctx, "gw:rl:t:a", p)
		require.NoError(t, err)
		require.False(t, d.Allowed)
		require.Greater(t, d.RetryAfter, time.Duration(0))
		require.LessOrEqual(t, d.RetryAfter, 100*time.Millisecond, "a 10/s una ficha tarda hasta 100 ms")
		time.Sleep(150 * time.Millisecond)
		d, err = store.Take(ctx, "gw:rl:t:a", p)
		require.NoError(t, err)
		require.True(t, d.Allowed, "recargó con el tiempo")
	})

	t.Run("claves independientes y vencimiento", func(t *testing.T) {
		p := Policy{Name: "t", Rate: 5.0 / 60, Burst: 1}
		d, _ := store.Take(ctx, "gw:rl:t:ip1", p)
		require.True(t, d.Allowed)
		d, _ = store.Take(ctx, "gw:rl:t:ip1", p)
		require.False(t, d.Allowed)
		require.InDelta(t, 12*time.Second, d.RetryAfter, float64(time.Second), "5 por minuto: una ficha cada 12 s")
		d, _ = store.Take(ctx, "gw:rl:t:ip2", p)
		require.True(t, d.Allowed, "otra IP no comparte balde")
		ttl, err := client.PTTL(ctx, "gw:rl:t:ip1").Result()
		require.NoError(t, err)
		require.Greater(t, ttl, time.Duration(0), "la clave vence sola")
	})

	t.Run("concurrencia: nunca más fichas que la ráfaga", func(t *testing.T) {
		p := Policy{Name: "t", Rate: 0.001, Burst: 20}
		var allowed atomic.Int64
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if d, err := store.Take(ctx, "gw:rl:t:conc", p); err == nil && d.Allowed {
					allowed.Add(1)
				}
			}()
		}
		wg.Wait()
		require.Equal(t, int64(20), allowed.Load(), "el script es atómico")
	})

	t.Run("Redis caído: el limitador falla abierto o cerrado según la política", func(t *testing.T) {
		dead := &Redis{Client: goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:1", MaxRetries: -1}), Timeout: 200 * time.Millisecond}
		l := New(dead, logger.NewWithWriter(&bytes.Buffer{}, "api-gateway", "error"))
		require.True(t, l.Allow(ctx, Policy{Name: "user", Rate: 10, Burst: 20}, "u").Allowed)
		d := l.Allow(ctx, Policy{Name: "login", Rate: 1, Burst: 5, FailClosed: true}, "ip")
		require.False(t, d.Allowed)
		require.True(t, d.Unavailable)
		require.Equal(t, int64(2), l.Metrics.RedisErrors.Load())
	})
}

func TestRetryAfterSeconds(t *testing.T) {
	require.Equal(t, 1, RetryAfterSeconds(0))
	require.Equal(t, 1, RetryAfterSeconds(10*time.Millisecond))
	require.Equal(t, 2, RetryAfterSeconds(1500*time.Millisecond))
	require.Equal(t, 12, RetryAfterSeconds(12*time.Second))
}
