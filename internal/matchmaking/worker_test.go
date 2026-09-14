package matchmaking

import (
	"context"
	"testing"
)

func TestWorkerRequiresDurableStoreAndAllocator(t *testing.T) {
	if matched, err := (Worker{}).RunOnce(context.Background()); err == nil || matched {
		t.Fatalf("expected configuration failure, matched=%v err=%v", matched, err)
	}
}
