package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/messaging"
	amqp "github.com/rabbitmq/amqp091-go"
)

// RelayMemberships publica solo eventos de activación (RN-05). Las otras
// routing keys quedan pendientes hasta que exista su consumidor; no se descartan.
// El bloqueo permanece hasta el confirm y el commit. Una caída posterior al
// confirm puede repetir el event_id: el consumidor debe deduplicar.
func (p *Postgres) RelayMemberships(ctx context.Context, bus *messaging.Session) (int, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	rows, err := tx.Query(ctx, `SELECT event_id::text,event_type,event_version,occurred_at,data FROM outbox_membresias WHERE publicado_en IS NULL AND event_type='membresia.activada' ORDER BY occurred_at,event_id LIMIT 100 FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return 0, err
	}
	type event struct {
		ID          string          `json:"event_id"`
		Type        string          `json:"event_type"`
		Version     int             `json:"event_version"`
		Occurred    time.Time       `json:"occurred_at"`
		Producer    string          `json:"producer"`
		Correlation string          `json:"correlation_id"`
		Traceparent string          `json:"traceparent"`
		Data        json.RawMessage `json:"data"`
	}
	var events []event
	for rows.Next() {
		var e event
		if err = rows.Scan(&e.ID, &e.Type, &e.Version, &e.Occurred, &e.Data); err != nil {
			rows.Close()
			return 0, err
		}
		events = append(events, e)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	for _, e := range events {
		e.Producer = "members-service"
		body, err := json.Marshal(e)
		if err != nil {
			return 0, err
		}
		publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = bus.Publish(publishCtx, "gym.events", e.Type, amqp.Publishing{ContentType: "application/json", MessageId: e.ID, Type: e.Type, Timestamp: e.Occurred, Body: body})
		cancel()
		if err != nil {
			return 0, err
		}
		if _, err = tx.Exec(ctx, `UPDATE outbox_membresias SET publicado_en=now() WHERE event_id=$1`, e.ID); err != nil {
			return 0, err
		}
	}
	return len(events), tx.Commit(ctx)
}
