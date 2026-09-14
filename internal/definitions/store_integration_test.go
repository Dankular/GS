//go:build integration

package definitions

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Dankular/GameService/internal/compiler"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDefinitionStorePublishAndActivate(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is required for integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	report, err := compiler.Compile(strings.NewReader(`apiVersion: game.platform/v1alpha1
kind: GameDefinition
metadata: {gameId: integration-definition, revision: 1}
spec: {catalog: {currencies: [{id: coins, precision: 0, minBalance: 0, maxBalance: 1000}]}}
`))
	if err != nil {
		t.Fatal(err)
	}
	store := Store{Pool: pool}
	ctx := context.Background()
	if err := store.Publish(ctx, report, "source", "integration-admin", "integration publish", true); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate(ctx, "integration-definition", "test", 1, "integration-admin", "integration activate", true); err != nil {
		t.Fatal(err)
	}
	if revision, err := store.Active(ctx, "integration-definition", "test"); err != nil || revision != 1 {
		t.Fatalf("unexpected active revision %d: %v", revision, err)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM platform.definition_activations WHERE game_id='integration-definition'`)
	_, _ = pool.Exec(ctx, `DELETE FROM platform.definition_revisions WHERE game_id='integration-definition'`)
	_, _ = pool.Exec(ctx, `DELETE FROM platform.environments WHERE game_id='integration-definition'`)
	_, _ = pool.Exec(ctx, `DELETE FROM platform.games WHERE game_id='integration-definition'`)
}
