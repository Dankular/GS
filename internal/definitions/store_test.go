package definitions

import (
	"context"
	"testing"

	"github.com/Dankular/GameService/internal/compiler"
)

func TestDefinitionStoreRequiresAuthorizationAndConfiguration(t *testing.T) {
	if err := (Store{}).Publish(context.Background(), compiler.Report{}, "", "actor", false); err != ErrUnauthorized {
		t.Fatalf("expected authorization error, got %v", err)
	}
	if err := (Store{}).Activate(context.Background(), "game", "prod", 1, "actor", false); err != ErrUnauthorized {
		t.Fatalf("expected authorization error, got %v", err)
	}
}
