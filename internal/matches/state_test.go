package matches

import (
	"testing"
	"time"
)

func TestMatchLifecycle(t *testing.T) {
	state := Queued
	for _, event := range []string{"matched", "allocate", "ready", "start", "finalize", "complete"} {
		var err error
		state, err = Transition(state, event)
		if err != nil {
			t.Fatal(err)
		}
	}
	if state != Completed {
		t.Fatal(state)
	}
}
func TestInvalidMatchTransition(t *testing.T) {
	if _, err := Transition(Completed, "start"); err == nil {
		t.Fatal("expected terminal transition rejection")
	}
}

func TestJoinClaimRoundTripAndContextValidation(t *testing.T) {
	publicKey, privateKey, err := NewKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	claim := JoinClaim{Issuer: "gameservice", Audience: "game-server", Subject: "player-1", MatchID: "match-1", AllocationID: "alloc-1", ServerBuild: "sha256:build", Slot: 2, IssuedAt: now.Unix(), NotBefore: now.Unix() - 1, ExpiresAt: now.Add(time.Minute).Unix(), JTI: "jti-1"}
	token, err := SignClaim(claim, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	got, err := VerifyClaim(token, publicKey, now, "game-server", "match-1", "sha256:build")
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != claim.Subject || got.Slot != claim.Slot {
		t.Fatalf("claim mismatch: %#v", got)
	}
	if _, err := VerifyClaim(token, publicKey, now, "wrong-audience", "match-1", "sha256:build"); err == nil {
		t.Fatal("expected audience mismatch")
	}
	if _, err := VerifyClaim(token, publicKey, now.Add(2*time.Minute), "game-server", "match-1", "sha256:build"); err == nil {
		t.Fatal("expected expiry rejection")
	}
}

func TestJoinClaimRejectsTampering(t *testing.T) {
	publicKey, privateKey, err := NewKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	claim := JoinClaim{Issuer: "gameservice", Audience: "game-server", Subject: "p", MatchID: "m", AllocationID: "a", ServerBuild: "b", IssuedAt: 10, NotBefore: 10, ExpiresAt: 20, JTI: "j"}
	token, err := SignClaim(claim, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	parts := []byte(token)
	parts[len(parts)-1] ^= 1
	if _, err := VerifyClaim(string(parts), publicKey, time.Unix(10, 0), "game-server", "m", "b"); err == nil {
		t.Fatal("expected tampered signature rejection")
	}
}
