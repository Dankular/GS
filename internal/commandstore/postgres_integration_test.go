//go:build integration

package commandstore

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Dankular/GameService/internal/commands"
)

func TestSubmitAndGetPersistsIdempotentResult(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is required for integration tests")
	}
	repo, err := New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	e := commands.Envelope{APIVersion: "game.platform/v1alpha1", Kind: "Command", Metadata: commands.Metadata{RequestID: "integration-request-" + suffix, CorrelationID: "integration-correlation-" + suffix, GameID: "game", Environment: "test", DefinitionRevision: 1}, Actor: commands.Actor{Type: "player", ID: "player"}, Spec: commands.Spec{Operation: "profile.get", Arguments: map[string]any{}}}
	result, replay, err := repo.Submit(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	if replay {
		t.Fatal("first submission was replayed")
	}
	loaded, err := repo.Get(context.Background(), e.Metadata.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RequestID != result.RequestID || loaded.Status != "succeeded" {
		t.Fatalf("loaded result mismatch: %#v", loaded)
	}
	if _, err := repo.GetForActor(context.Background(), e.Metadata.RequestID, "different-player"); err == nil {
		t.Fatal("different actor retrieved command result")
	}
	owned, err := repo.GetForActor(context.Background(), e.Metadata.RequestID, "player")
	if err != nil || owned.RequestID != e.Metadata.RequestID {
		t.Fatalf("owner could not retrieve command result: %#v, %v", owned, err)
	}
	_, replay, err = repo.Submit(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	if !replay {
		t.Fatal("second submission was not replayed")
	}
}
