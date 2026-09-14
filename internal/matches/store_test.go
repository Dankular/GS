package matches

import (
	"testing"
	"time"
)

func TestMatchStoreCreateRejectsIncompleteData(t *testing.T) {
	if _, err := (Store{}).Create(nil, MatchSpec{}, nil); err == nil {
		t.Fatal("expected incomplete match rejection")
	}
}

func TestMatchStoreRequiresSignerForClaims(t *testing.T) {
	if _, err := (Store{}).IssueJoinClaim(nil, "m", "p", nowForTest()); err == nil {
		t.Fatal("expected signer configuration error")
	}
}

func nowForTest() (now time.Time) { return time.Unix(1_700_000_000, 0) }
