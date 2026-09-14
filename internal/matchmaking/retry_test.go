package matchmaking

import "testing"

func TestAllocationFailureIsBounded(t *testing.T) {
	if got := allocationFailureStatus(0, 3); got != "queued" {
		t.Fatalf("first failure status = %s", got)
	}
	if got := allocationFailureStatus(1, 3); got != "queued" {
		t.Fatalf("second failure status = %s", got)
	}
	if got := allocationFailureStatus(2, 3); got != "expired" {
		t.Fatalf("third failure status = %s", got)
	}
	if got := allocationFailureStatus(0, 0); got != "queued" {
		t.Fatalf("default retry policy status = %s", got)
	}
}
