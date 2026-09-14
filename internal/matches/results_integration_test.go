//go:build integration

package matches

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSubmitResultPersistsAndDeduplicates(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is required for integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx := context.Background()
	matchID := "integration-result-" + time.Now().UTC().Format("20060102150405.000000000")
	if _, err := pool.Exec(ctx, `INSERT INTO match.matches(match_id,game_id,environment,mode_id,definition_revision,state,server_build) VALUES($1,'integration','test','dm',1,'Running','build')`, matchID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM match.matches WHERE match_id=$1`, matchID)
	payload := json.RawMessage(`{"score":10}`)
	digest := Digest(payload)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, _, err := SubmitResult(ctx, tx, ResultSubmission{MatchID: matchID, Sequence: 1, Payload: payload, PayloadDigest: digest})
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("first result was duplicate")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, _, err = SubmitResult(ctx, tx, ResultSubmission{MatchID: matchID, Sequence: 1, Payload: payload, PayloadDigest: digest})
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate {
		t.Fatal("second result was not duplicate")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := pool.QueryRow(ctx, `SELECT state FROM match.matches WHERE match_id=$1`, matchID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "Finalizing" {
		t.Fatalf("expected Finalizing, got %s", state)
	}
}
