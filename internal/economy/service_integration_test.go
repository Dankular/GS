//go:build integration

package economy

import (
	"context"
	"encoding/json"
	"os"
	"sync"
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
	canonical, err := json.Marshal(map[string]any{"apiVersion": "game.platform/v1alpha1", "kind": "GameDefinition", "metadata": map[string]any{"gameId": gameID, "revision": 1}, "spec": map[string]any{"catalog": map[string]any{"currencies": []any{map[string]any{"id": "coins", "precision": 0, "minBalance": 0, "maxBalance": 1000}}, "items": []any{map[string]any{"id": "badge", "stackLimit": 2}}}, "rewards": []any{map[string]any{"id": "welcome", "oncePerPlayer": true, "grants": []any{map[string]any{"currency": "coins", "amount": 25}, map[string]any{"item": "badge", "quantity": 1}}}}}})
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
	inputs := []commands.Envelope{envelope("reward-request-1-"+suffix, "source-1"), envelope("reward-request-2-"+suffix, "source-2")}
	for index, input := range inputs {
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
		if index == 0 {
			if result.Status != "succeeded" || result.Result["duplicate"] == true {
				t.Fatalf("first reward claim did not succeed: %#v", result)
			}
			var claims int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM economy.reward_claims WHERE player_id=$1 AND reward_id='welcome'`, playerID).Scan(&claims); err != nil {
				t.Fatal(err)
			}
			if claims != 1 {
				t.Fatalf("first reward claim was not persisted: %d", claims)
			}
		}
		if index == 1 && (result.Status != "succeeded" || result.Result["duplicate"] != true) {
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
	var ledgerEntries int
	var ledgerSum int64
	if err := pool.QueryRow(ctx, `SELECT count(*),COALESCE(sum(le.amount),0) FROM economy.ledger_entries le JOIN economy.ledger_transactions lt ON lt.request_id=le.request_id WHERE lt.reason='reward.claim' AND lt.request_id LIKE $1 AND le.player_id IN ($2,'__system__')`, "reward-request-1-"+suffix+":reward:%", playerID).Scan(&ledgerEntries, &ledgerSum); err != nil {
		t.Fatal(err)
	}
	if ledgerEntries != 2 || ledgerSum != 0 {
		t.Fatalf("reward ledger is not balanced: entries=%d sum=%d", ledgerEntries, ledgerSum)
	}

	concurrentPlayerID := playerID + "-concurrent"
	concurrentEnvelope := func(requestID, sourceID string) commands.Envelope {
		return commands.Envelope{Metadata: commands.Metadata{RequestID: requestID, CorrelationID: requestID, GameID: gameID, Environment: "test", DefinitionRevision: 1}, Actor: commands.Actor{ID: concurrentPlayerID}, Spec: commands.Spec{Operation: "reward.claim", Arguments: map[string]any{"rewardId": "welcome", "sourceId": sourceID}}}
	}
	results := make(chan commands.Result, 2)
	errorsCh := make(chan error, 2)
	var group sync.WaitGroup
	for index := 0; index < 2; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			tx, err := pool.Begin(ctx)
			if err != nil {
				errorsCh <- err
				return
			}
			result, err := (Service{}).Handle(ctx, tx, concurrentEnvelope("concurrent-"+suffix+"-"+string(rune('1'+index)), "concurrent-source-"+string(rune('1'+index))))
			if err == nil {
				err = tx.Commit(ctx)
			} else {
				_ = tx.Rollback(ctx)
			}
			if err != nil {
				errorsCh <- err
				return
			}
			results <- result
		}(index)
	}
	group.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		t.Fatal(err)
	}
	duplicates := 0
	for result := range results {
		if result.Result["duplicate"] == true {
			duplicates++
		}
	}
	if duplicates != 1 {
		t.Fatalf("concurrent once-per-player claims produced %d duplicates", duplicates)
	}
	var concurrentClaims int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM economy.reward_claims WHERE player_id=$1 AND reward_id='welcome'`, concurrentPlayerID).Scan(&concurrentClaims); err != nil {
		t.Fatal(err)
	}
	if concurrentClaims != 1 {
		t.Fatalf("concurrent reward claims persisted %d claims", concurrentClaims)
	}

	targetPlayerID := playerID + "-target"
	transfer := func(operation string, arguments map[string]any) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		result, err := (Service{}).Handle(ctx, tx, commands.Envelope{Metadata: commands.Metadata{RequestID: operation + "-" + suffix, CorrelationID: operation + "-" + suffix, GameID: gameID, Environment: "test", DefinitionRevision: 1}, Actor: commands.Actor{ID: playerID}, Spec: commands.Spec{Operation: operation, Arguments: arguments}})
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(err)
		}
		if result.Status != "succeeded" {
			_ = tx.Rollback(ctx)
			t.Fatalf("%s failed: code=%s message=%s result=%#v", operation, result.Error.Code, result.Error.Message, result)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	transfer("wallet.transfer", map[string]any{"targetPlayerId": targetPlayerID, "currency": "coins", "amount": json.Number("10")})
	transfer("inventory.transfer", map[string]any{"targetPlayerId": targetPlayerID, "itemId": "badge", "quantity": json.Number("1")})
	var sourceBalance, targetBalance, targetQuantity int64
	if err := pool.QueryRow(ctx, `SELECT balance FROM economy.wallet_accounts WHERE player_id=$1 AND currency='coins'`, playerID).Scan(&sourceBalance); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT balance FROM economy.wallet_accounts WHERE player_id=$1 AND currency='coins'`, targetPlayerID).Scan(&targetBalance); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT quantity FROM economy.inventory_stacks WHERE player_id=$1 AND item_id='badge'`, targetPlayerID).Scan(&targetQuantity); err != nil {
		t.Fatal(err)
	}
	if sourceBalance != 15 || targetBalance != 10 || targetQuantity != 1 {
		t.Fatalf("transfer results are incorrect: source=%d target=%d items=%d", sourceBalance, targetBalance, targetQuantity)
	}
}
