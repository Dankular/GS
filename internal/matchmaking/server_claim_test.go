package matchmaking

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/Dankular/GameService/internal/matches"
)

func TestServerClaimTokenIsBoundToAllocation(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	worker := Worker{ServerClaimPrivateKey: private, ServerClaimTTL: time.Minute}
	token, err := worker.serverClaimToken("match-1", "allocation-1", "sha256:build")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := matches.VerifyServerClaim(token, public, time.Now(), "match-1", "allocation-1", "sha256:build"); err != nil {
		t.Fatal(err)
	}
	if _, err := matches.VerifyServerClaim(token, public, time.Now(), "match-1", "allocation-2", "sha256:build"); err == nil {
		t.Fatal("expected allocation mismatch")
	}
}
