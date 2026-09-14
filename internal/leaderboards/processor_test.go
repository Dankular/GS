package leaderboards

import "testing"

import "github.com/Dankular/GameService/internal/compiler"

func TestDecodeResultPayloadRequiresPlayerScores(t *testing.T) {
	result, err := DecodeResultPayload([]byte(`{"players":[{"playerId":"p1","score":10}]}`))
	if err != nil || len(result.Players) != 1 || result.Players[0].Score != 10 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if _, err := DecodeResultPayload([]byte(`{"players":[{"score":10}]}`)); err == nil {
		t.Fatal("expected player ID validation")
	}
}

func TestConfiguredLeaderboardUsesDefinitionPolicy(t *testing.T) {
	definition := compiler.Definition{Spec: compiler.Spec{MatchModes: []compiler.MatchMode{{ID: "deathmatch", Rating: compiler.RatingPolicy{LeaderboardID: "arena_rating", Strategy: "authoritative"}}}}}
	if got := configuredLeaderboard(definition, "deathmatch"); got != "arena_rating" {
		t.Fatalf("configured leaderboard = %q", got)
	}
	if got := configuredLeaderboard(definition, "unknown"); got != "" {
		t.Fatalf("unknown mode returned leaderboard %q", got)
	}
}
