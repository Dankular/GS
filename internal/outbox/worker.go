package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Event struct {
	ID            string
	AggregateType string
	AggregateID   string
	EventType     string
	CorrelationID string
	Payload       json.RawMessage
	Attempts      int
}

type Publisher interface {
	Publish(context.Context, Event) error
}

type Worker struct {
	Pool        *pgxpool.Pool
	Publisher   Publisher
	Consumer    string
	BatchSize   int
	Lease       time.Duration
	MaxAttempts int
	Now         func() time.Time
}

func (w Worker) RunOnce(ctx context.Context) (int, error) {
	if w.Pool == nil || w.Publisher == nil || w.Consumer == "" {
		return 0, errors.New("outbox worker configuration is incomplete")
	}
	batchSize := w.BatchSize
	if batchSize <= 0 {
		batchSize = 50
	}
	lease := w.Lease
	if lease <= 0 {
		lease = time.Minute
	}
	maxAttempts := w.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	now := time.Now
	if w.Now != nil {
		now = w.Now
	}
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `
WITH claimed AS (
  SELECT event_id FROM ops.outbox_events
  WHERE published_at IS NULL AND dead_lettered_at IS NULL
    AND (leased_until IS NULL OR leased_until < $1)
  ORDER BY event_id FOR UPDATE SKIP LOCKED LIMIT $2
)
UPDATE ops.outbox_events e
SET leased_until=$1 + $3::interval, attempts=e.attempts+1
FROM claimed c WHERE e.event_id=c.event_id
RETURNING e.event_id::text,e.aggregate_type,e.aggregate_id,e.event_type,e.correlation_id,e.payload,e.attempts`, now(), batchSize, lease.String())
	if err != nil {
		return 0, fmt.Errorf("claim outbox events: %w", err)
	}
	var events []Event
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.AggregateType, &event.AggregateID, &event.EventType, &event.CorrelationID, &event.Payload, &event.Attempts); err != nil {
			rows.Close()
			return 0, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	for _, event := range events {
		w.deliver(ctx, event, maxAttempts, now())
	}
	return len(events), nil
}

func (w Worker) deliver(ctx context.Context, event Event, maxAttempts int, now time.Time) {
	err := w.Publisher.Publish(ctx, event)
	if err == nil {
		_, _ = w.Pool.Exec(ctx, `INSERT INTO ops.delivery_checkpoints(consumer_name,event_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, w.Consumer, event.ID)
		_, _ = w.Pool.Exec(ctx, `UPDATE ops.outbox_events SET published_at=$2,leased_until=NULL,last_error=NULL WHERE event_id=$1`, event.ID, now)
		return
	}
	message := err.Error()
	if event.Attempts >= maxAttempts {
		_, _ = w.Pool.Exec(ctx, `UPDATE ops.outbox_events SET dead_lettered_at=$2,leased_until=NULL,last_error=$3 WHERE event_id=$1`, event.ID, now, message)
		return
	}
	_, _ = w.Pool.Exec(ctx, `UPDATE ops.outbox_events SET leased_until=NULL,last_error=$2 WHERE event_id=$1`, event.ID, message)
}

type JSONPublisher struct{ Encoder *json.Encoder }

func (p JSONPublisher) Publish(_ context.Context, event Event) error {
	if p.Encoder == nil {
		return errors.New("JSON publisher encoder is nil")
	}
	return p.Encoder.Encode(event)
}
