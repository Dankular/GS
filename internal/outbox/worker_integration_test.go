//go:build integration

package outbox

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type integrationPublisher struct {
	remainingFailures int
	calls             int
}

func (p *integrationPublisher) Publish(_ context.Context, _ Event) error {
	p.calls++
	if p.remainingFailures > 0 {
		p.remainingFailures--
		return errors.New("simulated delivery failure")
	}
	return nil
}

func TestWorkerRetriesAndDeadLettersAfterBoundedAttempts(t *testing.T) {
	pool := integrationPool(t)
	defer pool.Close()
	ctx := context.Background()
	var eventID string
	err := pool.QueryRow(ctx, `INSERT INTO ops.outbox_events(aggregate_type,aggregate_id,event_type,correlation_id,payload) VALUES('test','outbox-retry','test.retry.v1','outbox-retry','{}'::jsonb) RETURNING event_id::text`).Scan(&eventID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(ctx, `DELETE FROM ops.outbox_events WHERE event_id=$1`, eventID) }()

	now := time.Date(2026, 9, 14, 20, 0, 0, 0, time.UTC)
	publisher := &integrationPublisher{remainingFailures: 2}
	worker := Worker{Pool: pool, Consumer: "integration-retry", Publisher: publisher, MaxAttempts: 2, Now: func() time.Time { return now }}
	if claimed, err := worker.RunOnce(ctx); err != nil || claimed != 1 {
		t.Fatalf("first run claimed=%d err=%v", claimed, err)
	}
	assertOutboxState(t, pool, eventID, 1, false, "simulated delivery failure")
	if claimed, err := worker.RunOnce(ctx); err != nil || claimed != 1 {
		t.Fatalf("second run claimed=%d err=%v", claimed, err)
	}
	assertOutboxState(t, pool, eventID, 2, true, "simulated delivery failure")
	if publisher.calls != 2 {
		t.Fatalf("publisher calls=%d, want 2", publisher.calls)
	}
	if claimed, err := worker.RunOnce(ctx); err != nil || claimed != 0 {
		t.Fatalf("dead-letter run claimed=%d err=%v", claimed, err)
	}
}

func TestWorkerTypedConsumerWritesCheckpointAndDeliversOnce(t *testing.T) {
	pool := integrationPool(t)
	defer pool.Close()
	ctx := context.Background()
	var eventID string
	err := pool.QueryRow(ctx, `INSERT INTO ops.outbox_events(aggregate_type,aggregate_id,event_type,correlation_id,payload) VALUES('test','outbox-checkpoint','test.checkpoint.v1','outbox-checkpoint','{}'::jsonb) RETURNING event_id::text`).Scan(&eventID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM ops.delivery_checkpoints WHERE event_id=$1`, eventID)
		_, _ = pool.Exec(ctx, `DELETE FROM ops.outbox_events WHERE event_id=$1`, eventID)
	}()

	publisher := &integrationPublisher{}
	worker := Worker{Pool: pool, Consumer: "integration-checkpoint", EventType: "test.checkpoint.v1", Publisher: publisher}
	if claimed, err := worker.RunOnce(ctx); err != nil || claimed != 1 {
		t.Fatalf("first run claimed=%d err=%v", claimed, err)
	}
	if claimed, err := worker.RunOnce(ctx); err != nil || claimed != 0 {
		t.Fatalf("checkpointed run claimed=%d err=%v", claimed, err)
	}
	if publisher.calls != 1 {
		t.Fatalf("publisher calls=%d, want 1", publisher.calls)
	}
	var checkpoints int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ops.delivery_checkpoints WHERE consumer_name=$1 AND event_id=$2`, worker.Consumer, eventID).Scan(&checkpoints); err != nil {
		t.Fatal(err)
	}
	if checkpoints != 1 {
		t.Fatalf("checkpoints=%d, want 1", checkpoints)
	}
}

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is required for integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	return pool
}

func assertOutboxState(t *testing.T, pool *pgxpool.Pool, eventID string, attempts int, deadLettered bool, lastError string) {
	t.Helper()
	var gotAttempts int
	var gotDeadLettered bool
	var gotError string
	if err := pool.QueryRow(context.Background(), `SELECT attempts, dead_lettered_at IS NOT NULL, last_error FROM ops.outbox_events WHERE event_id=$1`, eventID).Scan(&gotAttempts, &gotDeadLettered, &gotError); err != nil {
		t.Fatal(err)
	}
	if gotAttempts != attempts || gotDeadLettered != deadLettered || gotError != lastError {
		t.Fatalf("outbox state attempts=%d dead_lettered=%t last_error=%q, want attempts=%d dead_lettered=%t last_error=%q", gotAttempts, gotDeadLettered, gotError, attempts, deadLettered, lastError)
	}
}
