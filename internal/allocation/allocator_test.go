package allocation

import (
	"context"
	"testing"
)

func TestFakeAllocatorRequiresAllCompatibilityLabelsAndAllocatesOnce(t *testing.T) {
	selector := Selector{GameID: "arena", ModeID: "dm", Build: "sha256:build", Region: "eu-west", Protocol: "3"}
	allocator := NewFakeAllocator([]Server{{Name: "server-1", Address: "10.0.0.1", Ready: true, Ports: map[string]int{"game": 7000}, Labels: map[string]string{"platform.game/id": "arena", "platform.game/mode": "dm", "platform.game/build": "sha256:build", "platform.game/region": "eu-west", "platform.game/protocol": "3"}}})
	allocation, err := allocator.Allocate(context.Background(), selector)
	if err != nil {
		t.Fatal(err)
	}
	if allocation.GameServer != "server-1" || allocation.AllocationID == "" {
		t.Fatalf("unexpected allocation: %#v", allocation)
	}
	if _, err := allocator.Allocate(context.Background(), selector); err != ErrNoCompatibleServer {
		t.Fatalf("expected server reservation, got %v", err)
	}
}

func TestFakeAllocatorRejectsInvalidSelectorAndContext(t *testing.T) {
	allocator := NewFakeAllocator(nil)
	if _, err := allocator.Allocate(context.Background(), Selector{}); err == nil {
		t.Fatal("expected selector validation")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := allocator.Allocate(ctx, Selector{GameID: "g", ModeID: "m", Build: "b", Region: "r", Protocol: "p"}); err != context.Canceled {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
