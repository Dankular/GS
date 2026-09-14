package economy

import (
	"encoding/json"
	"testing"
)

func TestIntArgRejectsFractionsAndZero(t *testing.T) {
	if _, err := intArg(map[string]any{"amount": json.Number("1.5")}, "amount"); err == nil {
		t.Fatal("expected fractional amount rejection")
	}
	if _, err := intArg(map[string]any{"amount": json.Number("0")}, "amount"); err == nil {
		t.Fatal("expected zero rejection")
	}
}
