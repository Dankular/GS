package leaderboards

import "testing"

func TestDecodeResultPayloadRequiresPlayerScores(t *testing.T) {
	result, err := DecodeResultPayload([]byte(`{"players":[{"playerId":"p1","score":10}]}`))
	if err != nil || len(result.Players) != 1 || result.Players[0].Score != 10 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if _, err := DecodeResultPayload([]byte(`{"players":[{"score":10}]}`)); err == nil {
		t.Fatal("expected player ID validation")
	}
}
