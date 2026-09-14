package main

import (
	"testing"

	"github.com/Dankular/GameService/internal/auth"
)

func TestCommandScopeMatrix(t *testing.T) {
	checks := map[string]string{
		"wallet.get":            "player:read",
		"wallet.transfer":       "player:write",
		"matchmaking.enqueue":   "player:write",
		"match.get":             "player:read",
		"definition.activate":   "definition:activate",
		"admin.player_snapshot": "admin:read",
		"admin.audit_search":    "admin:read",
		"unknown.operation":     "",
	}
	for operation, expected := range checks {
		if got := commandScope(operation); got != expected {
			t.Errorf("commandScope(%q) = %q, want %q", operation, got, expected)
		}
	}
}

func TestSessionAllowsBaselinePlayerAccessWithoutNakamaScope(t *testing.T) {
	claims := auth.SessionClaims{}
	if !sessionAllowsScope(claims, "player:read") || !sessionAllowsScope(claims, "player:write") {
		t.Fatal("baseline player access was denied")
	}
	if sessionAllowsScope(claims, "admin:read") || sessionAllowsScope(auth.SessionClaims{Scope: "player:read"}, "player:write") {
		t.Fatal("privileged or unscopeable access was allowed")
	}
}
