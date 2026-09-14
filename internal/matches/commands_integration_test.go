//go:build integration

package matches

import (
	"context"
	"crypto/ed25519"
	"os"
	"testing"
	"time"

	"github.com/Dankular/GameService/internal/commands"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMatchCommandsEnforceOwnershipAndMutateTransactionally(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is required for integration tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	matchID := "integration-command-" + time.Now().UTC().Format("20060102150405.000000000")
	store := Store{Pool: pool, JoinPrivateKey: privateKey, Issuer: "control-plane", Audience: "game-server", ClaimTTL: time.Minute}
	if _, err := store.Create(ctx, MatchSpec{MatchID: matchID, GameID: "arena", Environment: "test", ModeID: "dm", DefinitionRevision: 1, Build: "build-1", AllocationID: "allocation-1"}, []RosterMember{{PlayerID: "owner", Slot: 0, Team: "red"}}); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM match.matches WHERE match_id=$1`, matchID)
	if _, err := pool.Exec(ctx, `UPDATE match.matches SET state='Ready' WHERE match_id=$1`, matchID); err != nil {
		t.Fatal(err)
	}
	service := CommandService{Store: store}
	envelope := func(operation, actor, request string, args map[string]any) commands.Envelope {
		return commands.Envelope{Metadata: commands.Metadata{RequestID: request, CorrelationID: request}, Actor: commands.Actor{Type: "player", ID: actor}, Spec: commands.Spec{Operation: operation, Arguments: args}}
	}
	get := envelope("match.get", "owner", "get-"+matchID, map[string]any{"matchId": matchID})
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Handle(ctx, tx, get)
	if err != nil || result.Status != "succeeded" {
		t.Fatalf("owned get failed: %#v %v", result, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	claim := envelope("match.issue_join_claim", "owner", "claim-"+matchID, map[string]any{"matchId": matchID})
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err = service.Handle(ctx, tx, claim)
	if err != nil || result.Status != "succeeded" || result.Result["token"] == "" {
		t.Fatalf("claim failed: %#v %v", result, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	abandon := envelope("match.abandon", "owner", "abandon-"+matchID, map[string]any{"matchId": matchID})
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err = service.Handle(ctx, tx, abandon)
	if err != nil || result.Status != "succeeded" {
		t.Fatalf("abandon failed: %#v %v", result, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := pool.QueryRow(ctx, `SELECT state FROM match.matches WHERE match_id=$1`, matchID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != string(Abandoned) {
		t.Fatalf("expected abandoned state, got %s", state)
	}

	foreign := envelope("match.get", "other", "foreign-"+matchID, map[string]any{"matchId": matchID})
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err = service.Handle(ctx, tx, foreign)
	if err != nil || result.Status != "rejected" || result.Error.Code != "MATCH_NOT_FOUND" {
		t.Fatalf("foreign get was not rejected: %#v %v", result, err)
	}
	_ = tx.Rollback(ctx)
}
