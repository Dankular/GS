package economy

import (
	"context"
	"testing"
)

func TestReadQueriesFailClosedWithoutPoolOrPlayer(t *testing.T) {
	if _, err := Wallets(context.Background(), nil, "player"); err == nil {
		t.Fatal("expected wallet query configuration error")
	}
	if _, err := Inventory(context.Background(), nil, "player"); err == nil {
		t.Fatal("expected inventory query configuration error")
	}
	if _, err := Snapshot(context.Background(), nil, "player"); err == nil {
		t.Fatal("expected snapshot query configuration error")
	}
}
