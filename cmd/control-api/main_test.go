package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Dankular/GameService/internal/commands"
)

func TestLoadEd25519PrivateKeyFromFile(t *testing.T) {
	_, expected, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "join-claim-key")
	if err := os.WriteFile(path, []byte(base64.RawStdEncoding.EncodeToString(expected)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadEd25519PrivateKey(path, "invalid-fallback")
	if err != nil || string(got) != string(expected) {
		t.Fatalf("file key load failed: %v", err)
	}
}

func TestCommandAuthorizationUsesOperationDefinitions(t *testing.T) {
	cases := []struct {
		operation, scope, actor string
	}{
		{"inventory.list", "player:read", "player"},
		{"wallet.credit", "player:write", "player"},
		{"match.submit_result", "server:write", "server"},
		{"definition.activate", "definition:activate", "admin"},
		{"admin.execute_command", "admin:write", "admin"},
	}
	for _, tc := range cases {
		if got := commandScope(tc.operation); got != tc.scope {
			t.Errorf("%s scope=%q, want %q", tc.operation, got, tc.scope)
		}
		if got := commandActorType(tc.operation); got != tc.actor {
			t.Errorf("%s actor=%q, want %q", tc.operation, got, tc.actor)
		}
		if _, ok := commands.DefinitionFor(tc.operation); !ok {
			t.Errorf("%s missing operation definition", tc.operation)
		}
	}
	if commandScope("unknown.operation") != "" || commandActorType("unknown.operation") != "" {
		t.Fatal("unknown operation received authorization metadata")
	}
}

func TestLoadEd25519PrivateKeyFallbackAndValidation(t *testing.T) {
	_, expected, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := loadEd25519PrivateKey("", base64.RawStdEncoding.EncodeToString(expected))
	if err != nil || string(got) != string(expected) {
		t.Fatalf("fallback key load failed: %v", err)
	}
	if _, err := loadEd25519PrivateKey("", "bad"); err == nil {
		t.Fatal("invalid key was accepted")
	}
}

func TestServeHTTPGracefullyShutsDownOnSignal(t *testing.T) {
	server := &http.Server{}
	signals := make(chan os.Signal, 1)
	started := make(chan struct{})
	blocked := make(chan struct{})
	defer close(blocked)
	listen := func() error {
		close(started)
		<-blocked
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- serveHTTP(server, signals, time.Second, listen) }()
	<-started
	signals <- os.Interrupt
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("graceful shutdown failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("graceful shutdown did not complete")
	}
}
