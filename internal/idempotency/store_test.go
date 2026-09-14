package idempotency

import "testing"

func TestStoreReturnsSameResult(t *testing.T) {
	s := New[string]()
	first, existing := s.GetOrPut("r", func() string { return "one" })
	if existing || first != "one" {
		t.Fatal(first, existing)
	}
	second, existing := s.GetOrPut("r", func() string { return "two" })
	if !existing || second != "one" {
		t.Fatal(second, existing)
	}
}
