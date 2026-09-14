package economy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Dankular/GameService/internal/commands"
	"github.com/Dankular/GameService/internal/compiler"
)

func TestIntArgRejectsFractionsAndZero(t *testing.T) {
	if _, err := intArg(map[string]any{"amount": json.Number("1.5")}, "amount"); err == nil {
		t.Fatal("expected fractional amount rejection")
	}
	if _, err := intArg(map[string]any{"amount": json.Number("0")}, "amount"); err == nil {
		t.Fatal("expected zero rejection")
	}
	if _, err := intArg(map[string]any{"amount": json.Number("1000000001")}, "amount"); err == nil {
		t.Fatal("expected maximum amount rejection")
	}
}

func TestResolvePlayerBindsPlayerActor(t *testing.T) {
	if _, err := resolvePlayer(map[string]any{"playerId": "other"}, commands.Actor{Type: "player", ID: "self"}); err == nil {
		t.Fatal("expected player target mismatch to be rejected")
	}
	if got, err := resolvePlayer(map[string]any{"playerId": "target"}, commands.Actor{Type: "admin", ID: "operator"}); err != nil || got != "target" {
		t.Fatalf("admin target was not preserved: got=%q err=%v", got, err)
	}
}

func TestBalancedLedgerEntriesOffsetPlayerDelta(t *testing.T) {
	for _, amount := range []int64{-25, 100} {
		entries := balancedLedgerEntries("player", amount)
		if len(entries) != 2 || entries[0].Account != "player" || entries[1].Account != systemLedgerAccount {
			t.Fatalf("unexpected ledger entries: %#v", entries)
		}
		if entries[0].Amount+entries[1].Amount != 0 {
			t.Fatalf("ledger entries are not balanced: %#v", entries)
		}
	}
}

func TestTransferTargetRejectsSelfTransfer(t *testing.T) {
	if _, err := transferTarget(map[string]any{"targetPlayerId": "player"}, "player"); err == nil {
		t.Fatal("expected self-transfer rejection")
	}
}

func TestCatalogLookupAndBusinessErrorCodes(t *testing.T) {
	definition := compiler.Definition{Spec: compiler.Spec{Catalog: compiler.Catalog{Currencies: []compiler.Currency{{ID: "coins", MinBalance: 0, MaxBalance: 100}}, Items: []compiler.Item{{ID: "badge", StackLimit: 1}}}}}
	if currency, ok := findCurrency(definition, "coins"); !ok || currency.MaxBalance != 100 {
		t.Fatal("currency catalog lookup failed")
	}
	if item, ok := findItem(definition, "badge"); !ok || item.StackLimit != 1 {
		t.Fatal("item catalog lookup failed")
	}
	for input, expected := range map[string]string{"unknown currency: gems": "UNKNOWN_CURRENCY", "unknown item: sword": "UNKNOWN_ITEM", "item stack limit exceeded": "STACK_LIMIT"} {
		if code, ok := businessErrorCode(errors.New(input)); !ok || code != expected {
			t.Fatalf("error %q mapped to %q, %v", input, code, ok)
		}
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
	if result.Error.Message != "profile operations are owned by Nakama and must be submitted through the Control API" {
		t.Fatalf("expected explicit Nakama routing boundary, got %q", result.Error.Message)
	}
}

func TestBusinessValidationErrorsBecomeDurableRejections(t *testing.T) {
	result, err := (Service{}).Handle(context.Background(), nil, commands.Envelope{Metadata: commands.Metadata{RequestID: "r", CorrelationID: "c"}, Actor: commands.Actor{ID: "p"}, Spec: commands.Spec{Operation: "wallet.credit", Arguments: map[string]any{}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "rejected" || result.Error == nil || result.Error.Code != "INVALID_ARGUMENT" {
		t.Fatalf("unexpected rejection: %#v", result)
	}
}
