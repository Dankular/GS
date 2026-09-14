//go:build integration

package economy

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Dankular/GameService/internal/commands"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestServiceRewardClaimIsAtomicAndOncePerPlayer(t *testing.T) {
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
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	gameID, playerID := "economy-game-"+suffix, "economy-player-"+suffix
	canonical, err := json.Marshal(map[string]any{"apiVersion": "game.platform/v1alpha1", "kind": "GameDefinition", "metadata": map[string]any{"gameId": gameID, "revision": 1}, "spec": map[string]any{"rewards": []any{map[string]any{"id": "welcome", "oncePerPlayer": true, "grants": []any{map[string]any{"currency": "coins", "amount": 25}, map[string]any{"item": "badge", "quantity": 1}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO platform.games(game_id) VALUES($1)`, gameID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM platform.games WHERE game_id=$1`, gameID)
	if _, err := pool.Exec(ctx, `INSERT INTO platform.definition_revisions(game_id,revision,digest,source_yaml,canonical,compiled,validation_report,actor_id,status) VALUES($1,1,'sha256:integration','{}',$2,$2,'{}','test','published')`, gameID, canonical); err != nil {
		t.Fatal(err)
	}
	envelope := func(requestID, sourceID string) commands.Envelope {
		return commands.Envelope{Metadata: commands.Metadata{RequestID: requestID, CorrelationID: requestID, GameID: gameID, Environment: "test", DefinitionRevision: 1}, Actor: commands.Actor{ID: playerID}, Spec: commands.Spec{Operation: "reward.claim", Arguments: map[string]any{"rewardId": "welcome", "sourceId": sourceID}}}
	}
	service := Service{}
	for _, input := range []commands.Envelope{envelope("reward-request-1", "source-1"), envelope("reward-request-2", "source-2")} {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		result, err := service.Handle(ctx, tx, input)
		if err != nil {
			tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if input.Metadata.RequestID == "reward-request-2" && (result.Status != "succeeded" || result.Result["duplicate"] != true) {
			t.Fatalf("expected duplicate reward result, got %#v", result)
		}
	}
	var balance, quantity int64
	if err := pool.QueryRow(ctx, `SELECT balance FROM economy.wallet_accounts WHERE player_id=$1 AND currency='coins'`, playerID).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT quantity FROM economy.inventory_stacks WHERE player_id=$1 AND item_id='badge'`, playerID).Scan(&quantity); err != nil {
		t.Fatal(err)
	}
	if balance != 25 || quantity != 1 {
		t.Fatalf("reward was not applied once: balance=%d quantity=%d", balance, quantity)
	}
}
