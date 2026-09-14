package commandstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Dankular/GameService/internal/commands"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

type Handler func(context.Context, pgx.Tx, commands.Envelope) (commands.Result, error)

func New(ctx context.Context, databaseURL string) (*Repository, error) {
	if databaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Repository{pool: pool}, nil
}

func (r *Repository) Close() { r.pool.Close() }

// Submit stores a command result and its outbox event atomically. A duplicate
// request returns the original result and does not append another event.
func (r *Repository) Submit(ctx context.Context, e commands.Envelope) (commands.Result, bool, error) {
	return r.SubmitWith(ctx, e, nil)
}

func (r *Repository) SubmitWith(ctx context.Context, e commands.Envelope, handler Handler) (commands.Result, bool, error) {
	if handler == nil {
		handler = func(context.Context, pgx.Tx, commands.Envelope) (commands.Result, error) {
			return commands.Result{Result: map[string]any{"accepted": true}}, nil
		}
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return commands.Result{}, false, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	var stored []byte
	err = tx.QueryRow(ctx, `INSERT INTO platform.command_requests
 (request_id, correlation_id, game_id, environment, definition_revision, actor_type, actor_id, operation, status, result)
	 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'pending','{}'::jsonb)
 ON CONFLICT (request_id) DO NOTHING RETURNING result`, e.Metadata.RequestID, e.Metadata.CorrelationID, e.Metadata.GameID, e.Metadata.Environment, e.Metadata.DefinitionRevision, e.Actor.Type, e.Actor.ID, e.Spec.Operation).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = tx.QueryRow(ctx, `SELECT result FROM platform.command_requests WHERE request_id=$1`, e.Metadata.RequestID).Scan(&stored); err != nil {
			return commands.Result{}, true, fmt.Errorf("load duplicate command: %w", err)
		}
		var result commands.Result
		if err := json.Unmarshal(stored, &result); err != nil {
			return commands.Result{}, true, fmt.Errorf("decode result: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return commands.Result{}, true, fmt.Errorf("commit duplicate: %w", err)
		}
		return result, true, nil
	}
	if err != nil {
		return commands.Result{}, false, fmt.Errorf("reserve command: %w", err)
	}
	result, err := handler(ctx, tx, e)
	if err != nil {
		return commands.Result{}, false, err
	}
	if result.RequestID == "" {
		result.RequestID = e.Metadata.RequestID
	}
	if result.CorrelationID == "" {
		result.CorrelationID = e.Metadata.CorrelationID
	}
	if result.Operation == "" {
		result.Operation = e.Spec.Operation
	}
	if result.Status == "" {
		result.Status = "succeeded"
	}
	if len(result.Events) == 0 {
		result.Events = []string{"command.accepted.v1"}
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return commands.Result{}, false, fmt.Errorf("marshal result: %w", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE platform.command_requests SET status=$2,result=$3::jsonb WHERE request_id=$1`, e.Metadata.RequestID, result.Status, payload); err != nil {
		return commands.Result{}, false, fmt.Errorf("update command result: %w", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ops.outbox_events (aggregate_type, aggregate_id, event_type, correlation_id, payload) VALUES ('command',$1,'command.accepted.v1',$2,$3::jsonb)`, e.Metadata.RequestID, e.Metadata.CorrelationID, payload); err != nil {
		return commands.Result{}, false, fmt.Errorf("store outbox event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return commands.Result{}, false, fmt.Errorf("commit transaction: %w", err)
	}
	return result, false, nil
}
