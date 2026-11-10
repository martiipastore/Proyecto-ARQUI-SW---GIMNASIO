// Package messaging contiene transporte AMQP técnico; no conoce el dominio.
package messaging

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Session usa un canal de publicación exclusivo y serializa sus confirms.
type Session struct {
	Conn     *amqp.Connection
	Channel  *amqp.Channel
	mu       sync.Mutex
	confirms <-chan amqp.Confirmation
	returns  <-chan amqp.Return
}

func Dial(ctx context.Context, address string) (*Session, error) {
	var socket net.Conn
	c, err := amqp.DialConfig(address, amqp.Config{Heartbeat: 10 * time.Second, Dial: func(network, addr string) (net.Conn, error) {
		conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, addr)
		if err == nil {
			socket = conn
			_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		}
		return conn, err
	}})
	if err != nil {
		return nil, errors.New("RabbitMQ no disponible")
	}
	if socket != nil {
		_ = socket.SetDeadline(time.Time{})
	}
	ch, err := c.Channel()
	if err != nil {
		_ = c.Close()
		return nil, errors.New("canal AMQP no disponible")
	}
	s := &Session{Conn: c, Channel: ch}
	if err = ch.Confirm(false); err != nil {
		s.Close()
		return nil, errors.New("confirms AMQP no disponibles")
	}
	s.confirms = ch.NotifyPublish(make(chan amqp.Confirmation, 1))
	s.returns = ch.NotifyReturn(make(chan amqp.Return, 1))
	return s, nil
}

func (s *Session) Close() { _ = s.Channel.Close(); _ = s.Conn.Close() }

// Publish confirma persistencia y exige una ruta existente. Ante resultado
// ambiguo cierra la sesión: un confirm tardío no confirma el siguiente mensaje.
func (s *Session) Publish(ctx context.Context, exchange, key string, message amqp.Publishing) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	message.DeliveryMode = amqp.Persistent
	if err := s.Channel.PublishWithContext(ctx, exchange, key, true, false, message); err != nil {
		s.Close()
		return errors.New("publicación AMQP pendiente")
	}
	select {
	case confirmation, ok := <-s.confirms:
		if !ok || !confirmation.Ack {
			s.Close()
			return errors.New("publicación AMQP no confirmada")
		}
		select {
		case <-s.returns:
			return errors.New("publicación AMQP sin destino")
		default:
		}
		return nil
	case <-ctx.Done():
		s.Close()
		return ctx.Err()
	}
}

func (s *Session) Exchanges() error {
	for name, kind := range map[string]string{"gym.events": "topic", "gym.retry": "direct", "gym.dlx": "direct"} {
		if err := s.Channel.ExchangeDeclare(name, kind, true, false, false, false, nil); err != nil {
			return err
		}
	}
	return nil
}

// Topology declara una cola quorum y reintentos TTL que vuelven a su routing key.
func (s *Session) Topology(queue, key string, delays []time.Duration) error {
	if err := s.Exchanges(); err != nil {
		return err
	}
	for _, name := range []string{queue, queue + ".dlq"} {
		if _, err := s.Channel.QueueDeclare(name, true, false, false, false, amqp.Table{"x-queue-type": "quorum"}); err != nil {
			return err
		}
	}
	if err := s.Channel.QueueBind(queue, key, "gym.events", false, nil); err != nil {
		return err
	}
	if err := s.Channel.QueueBind(queue+".dlq", queue, "gym.dlx", false, nil); err != nil {
		return err
	}
	for i, delay := range delays {
		name := RetryQueue(queue, i)
		args := amqp.Table{"x-queue-type": "quorum", "x-overflow": "reject-publish", "x-dead-letter-strategy": "at-least-once", "x-message-ttl": int64(delay / time.Millisecond), "x-dead-letter-exchange": "gym.events", "x-dead-letter-routing-key": key}
		if _, err := s.Channel.QueueDeclare(name, true, false, false, false, args); err != nil {
			return err
		}
		if err := s.Channel.QueueBind(name, name, "gym.retry", false, nil); err != nil {
			return err
		}
	}
	return nil
}

func RetryQueue(queue string, index int) string {
	return queue + []string{".retry.10s", ".retry.1m", ".retry.10m"}[index]
}
