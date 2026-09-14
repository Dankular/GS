package main

import (
	"net/http/httptest"
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

func TestCommandActorTypeIsDerivedFromOperationScope(t *testing.T) {
	for _, operation := range []string{"wallet.credit", "inventory.grant", "matchmaking.enqueue"} {
		if got := commandActorType(operation); got != "player" {
			t.Fatalf("%s actor type = %q, want player", operation, got)
		}
	}
	for _, operation := range []string{"admin.execute_command", "admin.audit_search", "definition.publish", "definition.activate"} {
		if got := commandActorType(operation); got != "admin" {
			t.Fatalf("%s actor type = %q, want admin", operation, got)
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

func TestCompletedMatchAcceptsOnlyDuplicateResultCheck(t *testing.T) {
	for _, state := range []string{"Running", "Finalizing", "Completed"} {
		if !resultStateAccepts(state) {
			t.Fatalf("result state %q was rejected", state)
		}
	}
	for _, state := range []string{"Ready", "Failed", "Abandoned"} {
		if resultStateAccepts(state) {
			t.Fatalf("result state %q was accepted", state)
		}
	}
}

func TestPrivacyRequestIDAndHash(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/players/me/privacy/export", nil)
	req.Header.Set("Idempotency-Key", "privacy-1")
	if got, ok := privacyRequestID(httptest.NewRecorder(), req); !ok || got != "privacy-1" {
		t.Fatalf("privacy request id = %q, %v", got, ok)
	}
	if privacyHash("player-a") == privacyHash("player-b") || len(privacyHash("player-a")) != 64 {
		t.Fatal("privacy hash is not stable and non-identifying")
	}
	bad := httptest.NewRequest("POST", "/", nil)
	if _, ok := privacyRequestID(httptest.NewRecorder(), bad); ok {
		t.Fatal("missing idempotency key accepted")
	}
}

func TestProductionDefinitionEnvironmentsRequireFourEyes(t *testing.T) {
	for _, environment := range []string{"prod", "production", "PROD"} {
		if !requiresFourEyes(environment) {
			t.Fatalf("environment %q did not require approval", environment)
		}
	}
	for _, environment := range []string{"dev", "staging", ""} {
		if requiresFourEyes(environment) {
			t.Fatalf("environment %q unexpectedly required production approval", environment)
		}
	}
}
