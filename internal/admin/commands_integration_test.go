//go:build integration

package admin

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Dankular/GameService/internal/commands"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAdminCommandsReadSnapshotAndAudit(t *testing.T) {
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
	player := "admin-snapshot-" + time.Now().UTC().Format("20060102150405.000000000")
	if _, err := pool.Exec(ctx, `INSERT INTO economy.wallet_accounts(player_id,currency,balance) VALUES($1,'coins',42)`, player); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO economy.inventory_stacks(player_id,item_id,quantity,version) VALUES($1,'badge',2,1)`, player); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM economy.wallet_accounts WHERE player_id=$1`, player)
	defer pool.Exec(ctx, `DELETE FROM economy.inventory_stacks WHERE player_id=$1`, player)
	service := CommandService{}
	envelope := func(operation string, args map[string]any) commands.Envelope {
		return commands.Envelope{Metadata: commands.Metadata{RequestID: operation + player, CorrelationID: operation + player}, Actor: commands.Actor{Type: "admin", ID: "admin"}, Spec: commands.Spec{Operation: operation, Arguments: args}}
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Handle(ctx, tx, envelope("admin.player_snapshot", map[string]any{"playerId": player}))
	if err != nil || result.Status != "succeeded" {
		t.Fatalf("snapshot failed: %#v %v", result, err)
	}
	result, err = service.Handle(ctx, tx, envelope("admin.audit_search", map[string]any{"limit": 1}))
	if err != nil || result.Status != "succeeded" {
		t.Fatalf("audit search failed: %#v %v", result, err)
	}
	result, err = service.Handle(ctx, tx, envelope("admin.execute_command", map[string]any{"operation": "wallet.get", "targetPlayerId": player, "arguments": map[string]any{}}))
	if err != nil || result.Status != "rejected" || result.Error == nil || result.Error.Code != "NESTED_OPERATION_FORBIDDEN" {
		t.Fatalf("nested operation boundary failed: %#v %v", result, err)
	}
	result, err = service.Handle(ctx, tx, envelope("admin.player_restrict", map[string]any{"playerId": player, "kind": "queue", "reason": "test restriction"}))
	if err != nil || result.Status != "succeeded" {
		t.Fatalf("restriction failed: %#v %v", result, err)
	}
	var restricted bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform.player_restrictions WHERE player_id=$1 AND kind='queue')`, player).Scan(&restricted); err != nil || !restricted {
		t.Fatalf("restriction was not persisted: %v", err)
	}
	result, err = service.Handle(ctx, tx, envelope("admin.player_unrestrict", map[string]any{"playerId": player}))
	if err != nil || result.Status != "succeeded" {
		t.Fatalf("unrestriction failed: %#v %v", result, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
