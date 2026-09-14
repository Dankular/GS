package matches

import (
	"context"
	"testing"

	"github.com/Dankular/GameService/internal/commands"
)

func TestRequiredStringRejectsMissingMatchID(t *testing.T) {
	if _, err := requiredString(map[string]any{}, "matchId"); err == nil {
		t.Fatal("expected missing match ID error")
	}
}

func TestCommandServiceRequiresTransaction(t *testing.T) {
	_, err := (CommandService{}).Handle(context.Background(), nil, commands.Envelope{
		Actor: commands.Actor{ID: "player-1"},
		Spec:  commands.Spec{Operation: "match.get", Arguments: map[string]any{"matchId": "match-1"}},
	})
	if err == nil {
		t.Fatal("expected transaction error")
	}
}
