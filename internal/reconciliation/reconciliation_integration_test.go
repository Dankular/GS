//go:build integration

package reconciliation

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRecoverStaleAllocationsFailsMatchAndTicket(t *testing.T) {
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
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	matchID := "stale-match-" + suffix
	ticketID := "stale-ticket-" + suffix
	if _, err := pool.Exec(ctx, `INSERT INTO match.matches(match_id,game_id,environment,mode_id,definition_revision,state,server_build,updated_at) VALUES($1,'game','test','dm',1,'Allocating','build',now()-interval '10 minutes')`, matchID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO match.tickets(ticket_id,game_id,environment,mode_id,definition_revision,build,region,capacity,status,properties,expires_at,match_id) VALUES($1,'game','test','dm',1,'build','eu-west',1,'matched','{}'::jsonb,now()+interval '10 minutes',$2)`, ticketID, matchID); err != nil {
		pool.Exec(ctx, `DELETE FROM match.matches WHERE match_id=$1`, matchID)
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM match.tickets WHERE ticket_id=$1`, ticketID)
		_, _ = pool.Exec(ctx, `DELETE FROM match.matches WHERE match_id=$1`, matchID)
	}()

	recovered, err := RecoverStaleAllocations(ctx, pool, time.Now().UTC().Add(-5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if recovered != 1 {
		t.Fatalf("recovered=%d, want 1", recovered)
	}
	var state, ticketStatus string
	if err := pool.QueryRow(ctx, `SELECT state FROM match.matches WHERE match_id=$1`, matchID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM match.tickets WHERE ticket_id=$1`, ticketID).Scan(&ticketStatus); err != nil {
		t.Fatal(err)
	}
	if state != "Failed" || ticketStatus != "expired" {
		t.Fatalf("unexpected recovery state: match=%s ticket=%s", state, ticketStatus)
	}
	var events int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ops.outbox_events WHERE aggregate_id=$1 AND event_type='match.failed.v1'`, matchID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("failure event count=%d, want 1", events)
	}
}

func TestRecoverStaleRunningMatchesAbandonsMatch(t *testing.T) {
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
	matchID := "stale-running-" + time.Now().UTC().Format("20060102150405.000000000")
	ticketID := matchID + "-ticket"
	if _, err := pool.Exec(ctx, `INSERT INTO match.matches(match_id,game_id,environment,mode_id,definition_revision,state,server_build,updated_at) VALUES($1,'game','test','dm',1,'Running','build',now()-interval '10 minutes')`, matchID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO match.tickets(ticket_id,game_id,environment,mode_id,definition_revision,build,region,capacity,status,properties,expires_at,match_id) VALUES($1,'game','test','dm',1,'build','eu-west',1,'matched','{}'::jsonb,now()+interval '10 minutes',$2)`, ticketID, matchID); err != nil {
		_, _ = pool.Exec(ctx, `DELETE FROM match.matches WHERE match_id=$1`, matchID)
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM match.tickets WHERE ticket_id=$1`, ticketID)
		_, _ = pool.Exec(ctx, `DELETE FROM match.matches WHERE match_id=$1`, matchID)
	}()
	abandoned, err := RecoverStaleRunningMatches(ctx, pool, time.Now().UTC().Add(-5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if abandoned != 1 {
		t.Fatalf("abandoned=%d, want 1", abandoned)
	}
	var state, ticketStatus string
	if err := pool.QueryRow(ctx, `SELECT state FROM match.matches WHERE match_id=$1`, matchID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM match.tickets WHERE ticket_id=$1`, ticketID).Scan(&ticketStatus); err != nil {
		t.Fatal(err)
	}
	if state != "Abandoned" || ticketStatus != "expired" {
		t.Fatalf("unexpected recovery state: match=%s ticket=%s", state, ticketStatus)
	}
	var events int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ops.outbox_events WHERE aggregate_id=$1 AND event_type='match.abandoned.v1'`, matchID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("abandonment event count=%d, want 1", events)
	}
}
