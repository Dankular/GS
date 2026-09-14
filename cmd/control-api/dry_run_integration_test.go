//go:build integration

package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Dankular/GameService/internal/compiler"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDryRunImpactReportsActiveAndInFlightState(t *testing.T) {
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

	gameID := "dry-run-impact-" + time.Now().UTC().Format("20060102150405.000000000")
	canonical := []byte(`{"apiVersion":"game.platform/v1alpha1","kind":"GameDefinition","metadata":{"gameId":"` + gameID + `","revision":1},"spec":{"catalog":{}}}`)
	_, err = pool.Exec(ctx, `
		INSERT INTO platform.games(game_id) VALUES($1);
		INSERT INTO platform.environments(game_id,environment) VALUES($1,'test');
		INSERT INTO platform.definition_revisions(game_id,revision,digest,source_yaml,canonical,compiled,validation_report,actor_id,status)
		VALUES($1,1,'sha256:from','{}',$2,$2,$2,'integration','published');
		INSERT INTO platform.definition_activations(game_id,environment,revision,activated_by) VALUES($1,'test',1,'integration');
		INSERT INTO match.matches(match_id,game_id,environment,mode_id,definition_revision,state,server_build)
		VALUES($1,$1,'test','arena',1,'Running','sha256:build');
		INSERT INTO match.tickets(ticket_id,game_id,environment,mode_id,definition_revision,build,region,capacity,status,properties,expires_at)
		VALUES($1,$1,'test','arena',1,'sha256:build','eu-west',2,'matching','{}',now()+interval '1 hour');
	`, gameID, canonical)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM match.tickets WHERE ticket_id=$1`, gameID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM match.matches WHERE match_id=$1`, gameID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM platform.definition_activations WHERE game_id=$1`, gameID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM platform.definition_revisions WHERE game_id=$1`, gameID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM platform.environments WHERE game_id=$1`, gameID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM platform.games WHERE game_id=$1`, gameID)
	}()

	report, err := compiler.Compile(strings.NewReader(`apiVersion: game.platform/v1alpha1
kind: GameDefinition
metadata: {gameId: ` + gameID + `, revision: 2}
spec: {catalog: {}}
`))
	if err != nil {
		t.Fatal(err)
	}
	impact, err := dryRunImpact(ctx, pool, report, gameID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if impact["fromDigest"] != "sha256:from" || impact["diff"] == "no changes" {
		t.Fatalf("dry-run revision comparison missing: %#v", impact)
	}
	counts := impact["impact"].(map[string]any)
	if counts["runningMatches"] != int64(1) || counts["queuedTickets"] != int64(1) {
		t.Fatalf("unexpected impact counts: %#v", counts)
	}
}
