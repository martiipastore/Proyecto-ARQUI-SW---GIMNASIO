package consumer

import (
	"context"
	"testing"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/messaging"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// ADR-005/RN-05: la transferencia TTL también debe ser durable si el destino
// está indisponible. RabbitMQ real; se acortan los TTL y el intervalo interno
// de re-publicación únicamente en esta fixture (producción conserva el default).
func TestReintentoRetieneMensajeSinDestino(t *testing.T) {
	ctx := context.Background()
	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{Image: "rabbitmq:3-management", Env: map[string]string{"RABBITMQ_DEFAULT_USER": "fixture", "RABBITMQ_DEFAULT_PASS": "fixture", "RABBITMQ_SERVER_ADDITIONAL_ERL_ARGS": "+S 2:2 -rabbit dead_letter_worker_publisher_confirm_timeout 1000"}, ExposedPorts: []string{"5672/tcp"}, WaitingFor: wait.ForListeningPort("5672/tcp").WithStartupTimeout(90 * time.Second)}, Started: true})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	address, err := ctr.Endpoint(ctx, "")
	require.NoError(t, err)
	bus, err := messaging.Dial(ctx, "amqp://fixture:fixture@"+address+"/")
	require.NoError(t, err)
	defer bus.Close()
	const queue = "notification.test-retention"
	require.NoError(t, bus.Topology(queue, "membresia.activada", []time.Duration{time.Second, 2 * time.Second, 3 * time.Second}))
	require.NoError(t, bus.Channel.QueueUnbind(queue, "membresia.activada", "gym.events", nil))
	publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	require.NoError(t, bus.Publish(publishCtx, "gym.retry", messaging.RetryQueue(queue, 0), amqp.Publishing{Body: []byte("fixture"), MessageId: "retention-001"}))
	cancel()
	// Vence el TTL con gym.events sin ningún binding para esa routing key.
	time.Sleep(2 * time.Second)
	require.NoError(t, bus.Channel.QueueBind(queue, "membresia.activada", "gym.events", false, nil))
	require.Eventually(t, func() bool {
		d, ok, err := bus.Channel.Get(queue, false)
		if err != nil || !ok {
			return false
		}
		require.Equal(t, "retention-001", d.MessageId)
		require.NoError(t, d.Ack(false))
		return true
	}, 75*time.Second, 250*time.Millisecond, "el mensaje TTL se perdió sin destino")
}
