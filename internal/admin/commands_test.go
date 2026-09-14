package admin

import "testing"

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
