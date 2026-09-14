//go:build integration

package definitions

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Dankular/GameService/internal/compiler"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDefinitionApprovalRequiresDistinctSecondActor(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	gameID := "approval-integration-" + time.Now().UTC().Format("20060102150405.000000000")
	source := `apiVersion: game.platform/v1alpha1
kind: GameDefinition
metadata: {gameId: ` + gameID + `, revision: 1}
spec: {catalog: {}}
`
	report, err := compiler.Compile(strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO platform.games(game_id) VALUES($1);
		INSERT INTO platform.definition_revisions(game_id,revision,digest,source_yaml,canonical,compiled,validation_report,actor_id,status)
		VALUES($1,1,$2,$3,$4,$4,$4,'publisher','published')`, gameID, report.Digest, source, report.Canonical)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM platform.definition_approvals WHERE game_id=$1`, gameID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM platform.definition_revisions WHERE game_id=$1`, gameID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM platform.games WHERE game_id=$1`, gameID)
	}()

	store := Store{Pool: pool}
	first, err := store.RequestOrApprove(ctx, gameID, "production", 1, "author-a", "proposed release")
	if err != nil || first.Status != "pending" || first.RequestedBy != "author-a" {
		t.Fatalf("first approval request = %#v, %v", first, err)
	}
	if _, err := store.RequestOrApprove(ctx, gameID, "production", 1, "author-a", "self approve"); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("same actor approval error = %v, want ErrApprovalRequired", err)
	}
	second, err := store.RequestOrApprove(ctx, gameID, "production", 1, "reviewer-b", "reviewed release")
	if err != nil || second.Status != "approved" || second.ApprovedBy != "reviewer-b" {
		t.Fatalf("second approval = %#v, %v", second, err)
	}
	if valid, err := store.HasApprovedActivation(ctx, gameID, "production", 1, second.ID, "author-a"); err != nil || !valid {
		t.Fatalf("approved activation check for original actor = %v, %v", valid, err)
	}
	if valid, err := store.HasApprovedActivation(ctx, gameID, "production", 1, second.ID, "reviewer-b"); err != nil || valid {
		t.Fatalf("approved activation check for approver = %v, %v", valid, err)
	}
}
