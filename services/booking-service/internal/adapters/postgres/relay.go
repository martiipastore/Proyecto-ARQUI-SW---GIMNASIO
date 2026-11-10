package postgres

import (
	"context"
	"encoding/json"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/messaging"
	"github.com/jackc/pgx/v5"
	amqp "github.com/rabbitmq/amqp091-go"
	"time"
)

// PublicarOutbox mantiene bloqueadas las filas hasta el confirm y el commit.
// Las caídas después de publicar pueden repetir event_id; nunca se marca antes.
func (d *DB) PublicarOutbox(ctx context.Context, bus *messaging.Session) error {
	return d.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT event_id::text,event_type,event_version,occurred_at,COALESCE(correlation_id,''),data FROM outbox WHERE publicado_en IS NULL ORDER BY occurred_at,event_id LIMIT 100 FOR UPDATE SKIP LOCKED`)
		if err != nil {
			return err
		}
		type event struct {
			ID          string          `json:"event_id"`
			Type        string          `json:"event_type"`
			Version     int             `json:"event_version"`
			Occurred    time.Time       `json:"occurred_at"`
			Correlation string          `json:"correlation_id"`
			Producer    string          `json:"producer"`
			Traceparent string          `json:"traceparent"`
			Data        json.RawMessage `json:"data"`
		}
		var events []event
		for rows.Next() {
			var e event
			if err = rows.Scan(&e.ID, &e.Type, &e.Version, &e.Occurred, &e.Correlation, &e.Data); err != nil {
				rows.Close()
				return err
			}
			e.Producer = "booking-service"
			e.Occurred = e.Occurred.UTC()
			events = append(events, e)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		for _, e := range events {
			body, err := json.Marshal(e)
			if err != nil {
				return err
			}
			publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err = bus.Publish(publishCtx, "gym.events", e.Type, amqp.Publishing{ContentType: "application/json", MessageId: e.ID, Type: e.Type, CorrelationId: e.Correlation, Timestamp: e.Occurred, Body: body})
			cancel()
			if err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE outbox SET publicado_en=now() WHERE event_id=$1`, e.ID); err != nil {
				return err
			}
		}
		return nil
	})
}
