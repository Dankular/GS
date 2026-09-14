package telemetry

import (
	"context"
	"testing"
)

func TestSetupWithoutEndpointIsNoop(t *testing.T) {
	shutdown, err := Setup(context.Background(), "gameservice.test", "")
	if err != nil || shutdown == nil {
		t.Fatalf("shutdown configured=%t err=%v", shutdown != nil, err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func TestSetupRequiresServiceName(t *testing.T) {
	if _, err := Setup(context.Background(), "", ""); err == nil {
		t.Fatal("expected service-name validation")
	}
}
