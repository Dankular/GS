//go:build integration

package matchmaking

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Dankular/GameService/internal/allocation"
	"github.com/Dankular/GameService/internal/matches"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestWorkerCreatesAllocatedMatchFromQueuedTickets(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is required for integration tests")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	gameID := "worker-game-" + now.Format("20060102150405.000000000")
	players := []string{"worker-p1-" + now.Format("20060102150405.000000000"), "worker-p2-" + now.Format("20060102150405.000000000")}
	store := Store{Pool: pool}
	var ticketIDs []string
	for _, playerID := range players {
		record, err := store.Create(ctx, TicketRequest{GameID: gameID, Environment: "test", ModeID: "dm", DefinitionRevision: 1, Build: "build-1", Region: "eu-west", Capacity: 1, PlayerIDs: []string{playerID}, ExpiresAt: now.Add(5 * time.Minute)}, playerID, now)
		if err != nil {
			t.Fatal(err)
		}
		ticketIDs = append(ticketIDs, record.TicketID)
	}
	defer pool.Exec(ctx, `DELETE FROM match.tickets WHERE ticket_id=ANY($1)`, ticketIDs)

	allocator := allocation.NewFakeAllocator([]allocation.Server{{
		Name: "worker-server", Address: "10.0.0.5", Ports: map[string]int{"game": 7777}, Ready: true,
		Labels: map[string]string{"platform.game/id": gameID, "platform.game/mode": "dm", "platform.game/build": "build-1", "platform.game/region": "eu-west", "platform.game/protocol": "udp"},
	}})
	worker := Worker{Pool: pool, Allocator: allocator, MatchStore: matches.Store{Pool: pool}, Policy: Policy{TeamSize: 1, Teams: 2, RatingWindow: 0}, Protocol: "udp"}
	matched, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Fatal("expected a matched batch")
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM match.tickets WHERE ticket_id=$1`, ticketIDs[0]).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "matched" {
		t.Fatalf("expected matched ticket, got %s", status)
	}
	var state, address string
	if err := pool.QueryRow(ctx, `SELECT state,server_address FROM match.matches WHERE game_id=$1 AND server_build='build-1' ORDER BY created_at DESC LIMIT 1`, gameID).Scan(&state, &address); err != nil {
		t.Fatal(err)
	}
	if state != "Allocating" || address != "10.0.0.5" {
		t.Fatalf("unexpected match allocation: state=%s address=%s", state, address)
	}
}
