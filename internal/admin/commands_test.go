package admin

import (
	"testing"
	"time"
)

func TestAllowedNestedOperationBoundary(t *testing.T) {
	for _, operation := range []string{"wallet.credit", "inventory.grant", "reward.claim", "progression.add_xp"} {
		if !allowedNestedOperation(operation) {
			t.Errorf("expected %s to be allowed", operation)
		}
	}
	for _, operation := range []string{"admin.audit_search", "definition.publish", "profile.patch_public_fields", "wallet.get", "matchmaking.enqueue"} {
		if allowedNestedOperation(operation) {
			t.Errorf("expected %s to be denied", operation)
		}
	}
}

func TestRestrictionExpiryRequiresFutureRFC3339(t *testing.T) {
	if _, err := restrictionExpiry("not-a-time"); err == nil {
		t.Fatal("accepted malformed expiry")
	}
	if _, err := restrictionExpiry(time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)); err == nil {
		t.Fatal("accepted expired restriction")
	}
	if expiry, err := restrictionExpiry(time.Now().UTC().Add(time.Hour).Format(time.RFC3339)); err != nil || expiry == nil {
		t.Fatalf("future expiry rejected: %v", err)
	}
}
