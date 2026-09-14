package commands

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func validEnvelope() Envelope {
	return Envelope{APIVersion: "game.platform/v1alpha1", Kind: "Command", Metadata: Metadata{
		RequestID: "01JREQ", CorrelationID: "01JCOR", GameID: "arena", Environment: "dev", DefinitionRevision: 1,
	}, Actor: Actor{Type: "player", ID: "player-1"}, Spec: Spec{Operation: "inventory.list", Arguments: map[string]any{}}}
}

func TestEnvelopeValidation(t *testing.T) {
	if err := validEnvelope().Validate(); err != nil {
		t.Fatal(err)
	}
	bad := validEnvelope()
	bad.Spec.Operation = "sql.execute"
	if !errors.Is(bad.Validate(), ErrUnknownOperation) {
		t.Fatalf("expected unknown operation, got %v", bad.Validate())
	}
	bad = validEnvelope()
	bad.Metadata.DefinitionRevision = 0
	if err := bad.Validate(); err == nil {
		t.Fatal("expected revision validation")
	}
}

func TestDecodeStrictRejectsUnknownFields(t *testing.T) {
	_, err := DecodeStrict([]byte(`{"apiVersion":"game.platform/v1alpha1","kind":"Command","unknown":true}`))
	if err == nil {
		t.Fatal("expected strict decoding error")
	}
}

func TestDecodeStrictRejectsTrailingJSON(t *testing.T) {
	data := `{"apiVersion":"game.platform/v1alpha1","kind":"Command","metadata":{"requestId":"r","correlationId":"c","gameId":"g","environment":"dev","definitionRevision":1},"actor":{"type":"player","id":"p"},"spec":{"operation":"inventory.list","arguments":{}}} {"extra":true}`
	if _, err := DecodeStrict([]byte(data)); err == nil {
		t.Fatal("expected trailing JSON to be rejected")
	}
}

func TestEnvelopeRejectsUnknownOperationArguments(t *testing.T) {
	e := validEnvelope()
	e.Spec.Arguments = map[string]any{"currency": "coins", "unexpected": true}
	if err := e.Validate(); err == nil {
		t.Fatal("expected unknown operation argument to be rejected")
	}
}

func TestEnvelopeRejectsOversizedArguments(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
	}{
		{name: "string", args: map[string]any{"value": strings.Repeat("x", maxArgumentStringSize+1)}},
		{name: "collection", args: map[string]any{"value": make([]any, maxArgumentCollection+1)}},
		{name: "depth", args: map[string]any{"value": nestedArguments(maxArgumentDepth + 1)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := validEnvelope()
			e.Spec.Arguments = tc.args
			if err := e.Validate(); err == nil {
				t.Fatalf("expected %s bound to be rejected", tc.name)
			}
		})
	}
}

func TestEnvelopeRejectsNonFiniteNumbers(t *testing.T) {
	e := validEnvelope()
	e.Spec.Arguments = map[string]any{"value": json.Number("NaN")}
	if err := e.Validate(); err == nil {
		t.Fatal("expected non-finite number to be rejected")
	}
}

func nestedArguments(depth int) map[string]any {
	root := map[string]any{}
	current := root
	for index := 0; index < depth; index++ {
		next := map[string]any{}
		current["next"] = next
		current = next
	}
	return root
}
