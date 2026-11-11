// Etapa 9 contra Redis 7 real (testcontainers): copia, TTL, invalidación,
// copia ilegible y Redis caído (ADR-007, D-88).
package redis

import (
	"context"
	"os/exec"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/domain"
)

func TestActividadesRedisReal(t *testing.T) {
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
	c := &Actividades{Client: client, TTL: time.Minute, Timeout: time.Second, Metricas: &Metricas{}}
	require.NoError(t, c.Ping(ctx))
	items := []domain.Actividad{{ID: "a", Nombre: "Musculación", CapacidadMaxima: 50, Musculacion: true, Activa: true}, {ID: "b", Nombre: "Zumba", CapacidadMaxima: 30, Activa: false}}

	_, ok, err := c.Leer(ctx)
	require.NoError(t, err)
	require.False(t, ok, "vacía es miss")
	require.NoError(t, c.Guardar(ctx, items))
	leidas, ok, err := c.Leer(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, items, leidas)
	ttl, err := client.TTL(ctx, Clave).Result()
	require.NoError(t, err)
	require.Greater(t, ttl, 50*time.Second, "la copia vence según el TTL configurado")

	require.NoError(t, c.Invalidar(ctx))
	_, ok, err = c.Leer(ctx)
	require.NoError(t, err)
	require.False(t, ok, "invalidada es miss")

	require.NoError(t, client.Set(ctx, Clave, "{roto", time.Minute).Err())
	_, ok, err = c.Leer(ctx)
	require.NoError(t, err)
	require.False(t, ok, "una copia ilegible se trata como miss")
	require.EqualValues(t, 1, c.Metricas.Aciertos.Load())
	require.EqualValues(t, 3, c.Metricas.Fallos.Load())
	require.EqualValues(t, 1, c.Metricas.Invalidaciones.Load())

	caido := &Actividades{Client: goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 200 * time.Millisecond}), TTL: time.Minute, Timeout: 300 * time.Millisecond, Metricas: &Metricas{}}
	inicio := time.Now()
	_, ok, err = caido.Leer(ctx)
	require.Error(t, err)
	require.False(t, ok)
	require.Less(t, time.Since(inicio), time.Second, "Redis caído no demora la lectura más que el timeout")
	require.Error(t, caido.Ping(ctx))
	require.EqualValues(t, 1, caido.Metricas.Errores.Load())
}
