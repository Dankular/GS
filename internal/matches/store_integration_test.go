//go:build integration

package matches

import (
	"context"
	"crypto/ed25519"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMatchStoreLifecycleAndOneTimeJoinClaim(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is required for integration tests")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	matchID := "integration-store-" + time.Now().UTC().Format("20060102150405.000000000")
	ctx := context.Background()
	store := Store{Pool: pool, JoinPrivateKey: privateKey, Issuer: "control-plane", Audience: "game-server", ClaimTTL: time.Minute}
	_, err = store.Create(ctx, MatchSpec{
		MatchID: matchID, GameID: "arena", Environment: "test", ModeID: "dm",
		DefinitionRevision: 1, Build: "build-1", AllocationID: "allocation-1",
	}, []RosterMember{{PlayerID: "player-1", Slot: 0, Team: "red"}})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM match.matches WHERE match_id=$1`, matchID)

	if _, err := pool.Exec(ctx, `UPDATE match.matches SET state='Allocating' WHERE match_id=$1`, matchID); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkReady(ctx, matchID); err != nil {
		t.Fatal(err)
	}
	if err := store.Heartbeat(ctx, matchID); err != nil {
		t.Fatal(err)
	}

	token, err := store.IssueJoinClaim(ctx, matchID, "player-1", time.Unix(1_700_000_000, 0))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := VerifyClaim(token, publicKey, time.Unix(1_700_000_001, 0), "game-server", matchID, "player-1")
	if err != nil {
		t.Fatal(err)
	}
	if claim.AllocationID != "allocation-1" || claim.Slot != 0 || claim.Team != "red" {
		t.Fatalf("unexpected claim: %+v", claim)
	}
	if err := store.ConsumeClaim(ctx, claim.JTI, matchID, "player-1", time.Unix(1_700_000_010, 0)); err != nil {
		t.Fatal(err)
	}
	if err := store.ConsumeClaim(ctx, claim.JTI, matchID, "player-1", time.Unix(1_700_000_011, 0)); err != pgx.ErrNoRows {
		t.Fatalf("expected one-time claim rejection, got %v", err)
	}
}
