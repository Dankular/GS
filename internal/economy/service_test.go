package economy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Dankular/GameService/internal/commands"
)

func TestIntArgRejectsFractionsAndZero(t *testing.T) {
	if _, err := intArg(map[string]any{"amount": json.Number("1.5")}, "amount"); err == nil {
		t.Fatal("expected fractional amount rejection")
	}
	if _, err := intArg(map[string]any{"amount": json.Number("0")}, "amount"); err == nil {
		t.Fatal("expected zero rejection")
	}
}

func TestUnimplementedOperationIsRejected(t *testing.T) {
	result, err := (Service{}).Handle(context.Background(), nil, commands.Envelope{Metadata: commands.Metadata{RequestID: "r", CorrelationID: "c"}, Actor: commands.Actor{ID: "p"}, Spec: commands.Spec{Operation: "profile.get"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "rejected" || result.Error == nil || result.Error.Code != "UNSUPPORTED_OPERATION" {
		t.Fatalf("unexpected result: %#v", result)
	}
}
