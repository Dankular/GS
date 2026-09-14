package commandstore

import (
	"context"
	"testing"
)

func TestNewRequiresDatabaseURL(t *testing.T) {
	if _, err := New(context.Background(), ""); err == nil {
		t.Fatal("expected DATABASE_URL error")
	}
}
