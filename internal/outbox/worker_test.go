package outbox

import (
	"context"
	"testing"
)

func TestJSONPublisherRequiresEncoder(t *testing.T) {
	if err := (JSONPublisher{}).Publish(context.Background(), Event{}); err == nil {
		t.Fatal("expected missing encoder error")
	}
}

func TestWorkerRequiresConfiguration(t *testing.T) {
	if _, err := (Worker{}).RunOnce(context.Background()); err == nil {
		t.Fatal("expected configuration error")
	}
}
