package definitions

import (
	"context"
	"testing"

	"github.com/Dankular/GameService/internal/compiler"
)

func TestDefinitionStoreRequiresAuthorizationAndConfiguration(t *testing.T) {
	if err := (Store{}).Publish(context.Background(), compiler.Report{}, "", "actor", "reason", false); err != ErrUnauthorized {
		t.Fatalf("expected authorization error, got %v", err)
	}
	if err := (Store{}).Activate(context.Background(), "game", "prod", 1, "actor", "reason", false); err != ErrUnauthorized {
		t.Fatalf("expected authorization error, got %v", err)
	}
	if err := (Store{}).Rollback(context.Background(), "game", "prod", 1, "actor", "reason", false); err != ErrUnauthorized {
		t.Fatalf("expected rollback authorization error, got %v", err)
	}
	if err := (Store{}).Publish(context.Background(), compiler.Report{}, "", "actor", "", true); err != ErrReasonRequired {
		t.Fatalf("expected reason error, got %v", err)
	}
	if _, err := (Store{}).Audit(context.Background(), 10); err == nil {
		t.Fatal("expected audit configuration error")
	}
}
