//go:build integration

package matchmaking

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTicketStoreCreateGetCancel(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is required for integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	now := time.Now().UTC()
	request := TicketRequest{GameID: "integration", Environment: "test", ModeID: "dm", DefinitionRevision: 1, Build: "build", Region: "eu-west", Capacity: 1, PlayerIDs: []string{"integration-player"}, Properties: map[string]any{"latencyBucket": "low"}, ExpiresAt: now.Add(5 * time.Minute)}
	store := Store{Pool: pool}
	record, err := store.Create(context.Background(), request, "integration-player", now)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), record.TicketID, "integration-player")
	if err != nil {
		t.Fatal(err)
	}
	if got.Build != request.Build || got.Region != request.Region || got.Status != "queued" {
		t.Fatalf("unexpected ticket: %#v", got)
	}
	if err := store.Cancel(context.Background(), record.TicketID, "integration-player"); err != nil {
		t.Fatal(err)
	}
	got, err = store.Get(context.Background(), record.TicketID, "integration-player")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "cancelled" {
		t.Fatalf("expected cancelled, got %s", got.Status)
	}
}
