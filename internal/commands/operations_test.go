package commands

import "testing"

func TestEveryRegisteredOperationHasACompleteDefinition(t *testing.T) {
	if len(registry) != len(operationDefinitions) {
		t.Fatalf("registry and operation definitions differ: registry=%d definitions=%d", len(registry), len(operationDefinitions))
	}
	for operation := range registry {
		definition, ok := DefinitionFor(operation)
		if !ok {
			t.Errorf("operation %q has no definition", operation)
			continue
		}
		if definition.ActorType == "" || definition.Scope == "" || definition.InputSchema == "" || definition.OutputSchema == "" || definition.Isolation == "" || definition.Idempotency == "" || definition.RateLimitBucket == "" || definition.Event == "" || definition.AuditPolicy == "" || definition.MaxExecution <= 0 {
			t.Errorf("operation %q has incomplete definition: %#v", operation, definition)
		}
	}
}

func TestDefinitionForUnknownOperation(t *testing.T) {
	if _, ok := DefinitionFor("not-a-command"); ok {
		t.Fatal("unknown operation unexpectedly has a definition")
	}
}
