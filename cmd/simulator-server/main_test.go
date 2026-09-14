package main

import "testing"

func TestParseRosterBoundsInvalidEntries(t *testing.T) {
	roster := parseRoster("player-a:0,player-b:2,broken,player-c:-1")
	if len(roster) != 2 || roster["player-a"] != 0 || roster["player-b"] != 2 {
		t.Fatalf("unexpected roster: %#v", roster)
	}
}
