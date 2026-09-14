package commands

import (
	"errors"
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
