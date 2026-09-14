package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseRosterBoundsInvalidEntries(t *testing.T) {
	roster := parseRoster("player-a:0,player-b:2,broken,player-c:-1")
	if len(roster) != 2 || roster["player-a"] != 0 || roster["player-b"] != 2 {
		t.Fatalf("unexpected roster: %#v", roster)
	}
}

type heartbeatProbe struct {
	calls atomic.Uint64
}

func (p *heartbeatProbe) Heartbeat() error {
	calls := p.calls.Add(1)
	if calls == 1 {
		return errors.New("temporary failure")
	}
	return nil
}

func TestMaintainMatchHeartbeatRetriesAfterTransientFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	probe := &heartbeatProbe{}
	done := make(chan struct{})
	go func() {
		maintainMatchHeartbeatInterval(ctx, probe, time.Millisecond)
		close(done)
	}()
	deadline := time.After(time.Second)
	for probe.calls.Load() < 2 {
		select {
		case <-deadline:
			t.Fatal("heartbeat loop did not retry after transient failure")
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("heartbeat loop did not stop after cancellation")
	}
}
