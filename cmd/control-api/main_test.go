package main

import "testing"

func TestCommandScopeMatrix(t *testing.T) {
	checks := map[string]string{
		"wallet.get":            "player:read",
		"wallet.transfer":       "player:write",
		"matchmaking.enqueue":   "player:write",
		"match.get":             "player:read",
		"definition.activate":   "admin:write",
		"admin.player_snapshot": "admin:write",
		"unknown.operation":     "",
	}
	for operation, expected := range checks {
		if got := commandScope(operation); got != expected {
			t.Errorf("commandScope(%q) = %q, want %q", operation, got, expected)
		}
	}
}
