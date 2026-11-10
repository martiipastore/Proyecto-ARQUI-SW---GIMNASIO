package messaging

import (
	"context"
	"errors"
	amqp "github.com/rabbitmq/amqp091-go"
	"log/slog"
	"sync/atomic"
	"time"
)

type Route struct{ Queue, Key string }

var RetryDelays = []time.Duration{10 * time.Second, time.Minute, 10 * time.Minute}

// Relay solo coordina transporte y reconexión; la fuente transaccional es un
// callback del adaptador del servicio propietario del outbox.
type Relay struct {
	URL       string
	Routes    []Route
	Publish   func(context.Context, *Session) error
	Log       *slog.Logger
	Connected atomic.Bool
}

func (r *Relay) Ready(context.Context) error {
	if !r.Connected.Load() {
		return errors.New("relay RabbitMQ desconectado")
	}
	return nil
}
func (r *Relay) Run(ctx context.Context) {
	var bus *Session
	defer func() {
		r.Connected.Store(false)
		if bus != nil {
			bus.Close()
		}
	}()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			work, cancel := context.WithTimeout(ctx, 10*time.Second)
			var err error
			if bus != nil && bus.Conn.IsClosed() {
				bus.Close()
				bus = nil
				r.Connected.Store(false)
			}
			if bus == nil {
				bus, err = Dial(work, r.URL)
				if err == nil {
					for _, route := range r.Routes {
						if err = bus.Topology(route.Queue, route.Key, RetryDelays); err != nil {
							break
						}
					}
				}
			}
			if err == nil {
				r.Connected.Store(true)
				err = r.Publish(work, bus)
			}
			cancel()
			if err != nil {
				r.Connected.Store(false)
				if bus != nil {
					bus.Close()
					bus = nil
				}
				r.Log.Warn("publicación pendiente; se reintentará")
			}
		}
	}
}

type Consumer struct {
	URL                      string
	Route                    Route
	Handle                   func(context.Context, []byte) error
	Permanent                func(error) bool
	Log                      *slog.Logger
	Connected                atomic.Bool
	Processed, Retried, Dead atomic.Int64
}

func (c *Consumer) Ready(context.Context) error {
	if !c.Connected.Load() {
		return errors.New("consumidor RabbitMQ desconectado")
	}
	return nil
}
func (c *Consumer) Run(ctx context.Context) {
	for ctx.Err() == nil {
		if err := c.consume(ctx); err != nil && ctx.Err() == nil {
			c.Log.Warn("consumidor desconectado; se reintentará", slog.String("queue", c.Route.Queue))
		}
		c.Connected.Store(false)
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
func (c *Consumer) consume(ctx context.Context) error {
	bus, err := Dial(ctx, c.URL)
	if err != nil {
		return err
	}
	defer bus.Close()
	if err = bus.Topology(c.Route.Queue, c.Route.Key, RetryDelays); err != nil {
		return err
	}
	if err = bus.Channel.Qos(10, 0, false); err != nil {
		return err
	}
	deliveries, err := bus.Channel.Consume(c.Route.Queue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	c.Connected.Store(true)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case d, ok := <-deliveries:
			if !ok {
				return errors.New("canal AMQP cerrado")
			}
			work, cancel := context.WithTimeout(ctx, 15*time.Second)
			err = c.deliver(work, bus, d)
			cancel()
			if err != nil {
				return err
			}
		}
	}
}
func (c *Consumer) deliver(ctx context.Context, bus *Session, d amqp.Delivery) error {
	attempt := 0
	if value, ok := d.Headers["x-attempt"]; ok {
		switch n := value.(type) {
		case int32:
			attempt = int(n)
		case int64:
			attempt = int(n)
		case int:
			attempt = n
		default:
			attempt = 3
		}
		if attempt < 0 || attempt > 3 {
			attempt = 3
		}
	}
	err := c.Handle(ctx, d.Body)
	if err == nil {
		if err = d.Ack(false); err == nil {
			c.Processed.Add(1)
		}
		return err
	}
	msg := amqp.Publishing{ContentType: d.ContentType, MessageId: d.MessageId, Type: d.Type, CorrelationId: d.CorrelationId, Timestamp: d.Timestamp, Body: d.Body, Headers: amqp.Table{"x-attempt": int32(attempt + 1)}}
	exchange, key := "gym.dlx", c.Route.Queue
	dead := attempt >= 3 || (c.Permanent != nil && c.Permanent(err))
	if !dead {
		exchange, key = "gym.retry", RetryQueue(c.Route.Queue, attempt)
	}
	if err = bus.Publish(ctx, exchange, key, msg); err != nil {
		return err
	}
	if err = d.Ack(false); err != nil {
		return err
	}
	if dead {
		c.Dead.Add(1)
		c.Log.Warn("mensaje enviado a DLQ", slog.String("queue", c.Route.Queue))
	} else {
		c.Retried.Add(1)
	}
	return nil
}
