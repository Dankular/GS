//go:build integration

package definitions

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Dankular/GameService/internal/commands"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDefinitionCommandsValidatePublishAndActivate(t *testing.T) {
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
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	gameID := "definition-command-" + suffix
	source := `apiVersion: game.platform/v1alpha1
kind: GameDefinition
metadata: {gameId: ` + gameID + `, revision: 1}
spec: {catalog: {currencies: [{id: coins, precision: 0, minBalance: 0, maxBalance: 1000}]}}
`
	service := CommandService{Store: Store{Pool: pool}}
	envelope := func(operation string, args map[string]any) commands.Envelope {
		return commands.Envelope{Metadata: commands.Metadata{RequestID: "definition-command-" + operation + "-" + suffix, CorrelationID: "definition-correlation-" + suffix}, Actor: commands.Actor{Type: "admin", ID: "admin-1"}, Spec: commands.Spec{Operation: operation, Arguments: args}}
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Handle(ctx, tx, envelope("definition.validate", map[string]any{"source": source}))
	if err != nil || result.Status != "succeeded" {
		t.Fatalf("validate failed: %#v %v", result, err)
	}
	result, err = service.Handle(ctx, tx, envelope("definition.publish", map[string]any{"source": source, "reason": "integration publish"}))
	if err != nil || result.Status != "succeeded" {
		t.Fatalf("publish failed: %#v %v", result, err)
	}
	result, err = service.Handle(ctx, tx, envelope("definition.activate", map[string]any{"gameId": gameID, "environment": "test", "revision": json.Number("1"), "reason": "integration activate"}))
	if err != nil || result.Status != "succeeded" {
		t.Fatalf("activate failed: %#v %v", result, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var active int64
	if err := pool.QueryRow(ctx, `SELECT revision FROM platform.definition_activations WHERE game_id=$1 AND environment='test'`, gameID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("expected active revision 1, got %d", active)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM platform.definition_activations WHERE game_id=$1`, gameID)
		_, _ = pool.Exec(ctx, `DELETE FROM platform.definition_revisions WHERE game_id=$1`, gameID)
		_, _ = pool.Exec(ctx, `DELETE FROM platform.environments WHERE game_id=$1`, gameID)
		_, _ = pool.Exec(ctx, `DELETE FROM platform.games WHERE game_id=$1`, gameID)
	}()
}
