package matchmaking

import (
	"testing"
	"time"
)

func TestTicketValidationBindsActorAndTimeWindow(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	request := TicketRequest{GameID: "arena", Environment: "prod", ModeID: "dm", DefinitionRevision: 2, Build: "build", Region: "eu-west", Capacity: 2, PlayerIDs: []string{"player-1"}, ExpiresAt: now.Add(5 * time.Minute)}
	if err := request.Validate("player-1", now); err != nil {
		t.Fatal(err)
	}
	if err := request.Validate("other-player", now); err == nil {
		t.Fatal("expected actor membership rejection")
	}
	request.ExpiresAt = now.Add(31 * time.Minute)
	if err := request.Validate("player-1", now); err == nil {
		t.Fatal("expected expiry window rejection")
	}
}
