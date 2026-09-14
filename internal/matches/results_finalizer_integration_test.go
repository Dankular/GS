//go:build integration

package matches

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Dankular/GameService/internal/compiler"
	"github.com/Dankular/GameService/internal/economy"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestResultFinalizerCompletesMatchAndAppliesConfiguredReward(t *testing.T) {
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
	gameID, matchID := "finalizer-game-"+suffix, "finalizer-match-"+suffix
	source := `apiVersion: game.platform/v1alpha1
kind: GameDefinition
metadata: {gameId: ` + gameID + `, revision: 1}
spec:
  catalog: {currencies: [{id: coins, precision: 0, minBalance: 0, maxBalance: 1000}]}
  rewards: [{id: match_reward, oncePerPlayer: true, grants: [{currency: coins, amount: 25}]}]
  matchModes: [{id: deathmatch, minPlayers: 2, maxPlayers: 2, teamSize: 1, regions: [eu-west], fleetRef: deathmatch-v1, serverBuild: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef, resultPolicy: {schema: result, rewardId: match_reward}}]
`
	report, err := compiler.Compile(strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO platform.games(game_id) VALUES($1)`, gameID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO platform.definition_revisions(game_id,revision,digest,source_yaml,canonical,compiled,validation_report,actor_id,status) VALUES($1,1,$2,$3,$4::jsonb,$4::jsonb,$5::jsonb,'integration','activated')`, gameID, report.Digest, source, report.Canonical, mustMarshal(report)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO match.matches(match_id,game_id,environment,mode_id,definition_revision,state,server_build,allocation_id) VALUES($1,$2,'test','deathmatch',1,'Running',$3,'allocation')`, matchID, gameID, "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatal(err)
	}
	for slot, player := range []string{"finalizer-p1-" + suffix, "finalizer-p2-" + suffix} {
		if _, err := pool.Exec(ctx, `INSERT INTO match.roster_members(match_id,player_id,slot,team) VALUES($1,$2,$3,'red')`, matchID, player, slot); err != nil {
			t.Fatal(err)
		}
	}
	defer pool.Exec(ctx, `DELETE FROM match.matches WHERE match_id=$1`, matchID)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	finalizer := ResultFinalizer{Economy: economy.Service{}}
	payload := json.RawMessage(`{"players":[{"playerId":"` + "finalizer-p1-" + suffix + `","score":10},{"playerId":"` + "finalizer-p2-" + suffix + `","score":5}]}`)
	duplicate, _, err := finalizer.Submit(ctx, tx, ResultSubmission{MatchID: matchID, Sequence: 1, Payload: payload, CorrelationID: "finalizer-correlation-" + suffix})
	if err != nil || duplicate {
		t.Fatalf("finalizer submit failed: duplicate=%v err=%v", duplicate, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := pool.QueryRow(ctx, `SELECT state FROM match.matches WHERE match_id=$1`, matchID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "Completed" {
		t.Fatalf("expected Completed, got %s", state)
	}
	var claims, balance, events int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM economy.reward_claims WHERE source_id=$1`, "match:"+matchID+":1").Scan(&claims); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT balance FROM economy.wallet_accounts WHERE player_id=$1 AND currency='coins'`, "finalizer-p1-"+suffix).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ops.outbox_events WHERE aggregate_id=$1 AND event_type='match.completed.v1'`, matchID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if claims != 2 || balance != 25 || events != 1 {
		t.Fatalf("unexpected effects: claims=%d balance=%d events=%d", claims, balance, events)
	}
}

func mustMarshal(value any) []byte { data, _ := json.Marshal(value); return data }
