package matchmaking

import (
	"context"
	"testing"

	"github.com/Dankular/GameService/internal/commands"
)

func TestCommandArgumentDecoding(t *testing.T) {
	players, ok := stringSlice([]any{"p1", "p2"})
	if !ok || len(players) != 2 || players[1] != "p2" {
		t.Fatalf("unexpected players: %#v, %v", players, ok)
	}
	if _, ok := stringSlice([]any{"p1", 2}); ok {
		t.Fatal("accepted non-string player ID")
	}
}

func TestCommandServiceRequiresTransaction(t *testing.T) {
	_, err := (CommandService{}).Handle(context.Background(), nil, commands.Envelope{Spec: commands.Spec{Operation: "matchmaking.status"}})
	if err == nil {
		t.Fatal("expected transaction error")
	}
}
