package consumer

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/messaging"
	amqp "github.com/rabbitmq/amqp091-go"
)

const Queue = "notification.membresia-activada"

var Delays = []time.Duration{10 * time.Second, time.Minute, 10 * time.Minute}

type FailureStore interface {
	RecordFailure(context.Context, []byte, int) error
}

type Worker struct {
	URL       string
	Processor Processor
	Failures  FailureStore
	Log       *slog.Logger
	Connected atomic.Bool
	Processed atomic.Int64
	Retries   atomic.Int64
	Failed    atomic.Int64
}

func (w *Worker) Ready(context.Context) error {
	if !w.Connected.Load() {
		return errors.New("consumidor RabbitMQ desconectado")
	}
	return nil
}

func (w *Worker) Run(ctx context.Context) {
	for ctx.Err() == nil {
		if err := w.session(ctx); err != nil && ctx.Err() == nil {
			w.Log.Warn("consumidor desconectado; se reintentará")
		}
		w.Connected.Store(false)
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (w *Worker) session(ctx context.Context) error {
	bus, err := messaging.Dial(ctx, w.URL)
	if err != nil {
		return err
	}
	defer bus.Close()
	if err = bus.Topology(Queue, "membresia.activada", Delays); err != nil {
		return err
	}
	if err = bus.Channel.Qos(10, 0, false); err != nil {
		return err
	}
	deliveries, err := bus.Channel.Consume(Queue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	w.Connected.Store(true)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case d, ok := <-deliveries:
			if !ok {
				return errors.New("canal cerrado")
			}
			workCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			err = w.deliver(workCtx, bus, d)
			cancel()
			if err != nil {
				return err
			}
		}
	}
}

func (w *Worker) deliver(ctx context.Context, bus *messaging.Session, d amqp.Delivery) error {
	attempt := 0
	if h, ok := d.Headers["x-attempt"]; ok {
		switch n := h.(type) {
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
	err := w.Processor.Process(ctx, d.Body)
	if err == nil {
		if err = d.Ack(false); err == nil {
			w.Processed.Add(1)
		}
		return err
	}
	message := amqp.Publishing{ContentType: d.ContentType, MessageId: d.MessageId, Type: d.Type, CorrelationId: d.CorrelationId, Timestamp: d.Timestamp, Body: d.Body, Headers: amqp.Table{"x-attempt": int32(attempt + 1)}}
	if errors.Is(err, ErrInvalid) || attempt >= 3 {
		if err = w.Failures.RecordFailure(ctx, d.Body, attempt+1); err != nil {
			return err
		}
		if err = bus.Publish(ctx, "gym.dlx", Queue, message); err != nil {
			return err
		}
		if err = d.Ack(false); err == nil {
			w.Failed.Add(1)
			w.Log.Warn("mensaje enviado a DLQ", slog.Int("intentos", attempt+1))
		}
		return err
	}
	if err = bus.Publish(ctx, "gym.retry", messaging.RetryQueue(Queue, attempt), message); err != nil {
		return err
	}
	if err = d.Ack(false); err == nil {
		w.Retries.Add(1)
		w.Log.Info("notificación programada para reintento", slog.Int("intento", attempt+1))
	}
	return err
}
