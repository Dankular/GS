package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadServerClaimPrivateKeyFromFile(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "server-claim-key")
	encoded := base64.RawStdEncoding.EncodeToString(privateKey)
	if err := os.WriteFile(path, []byte(encoded+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadServerClaimPrivateKey(path, "not-used")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(privateKey) {
		t.Fatal("file key was not loaded")
	}
}

func TestLoadServerClaimPrivateKeyFallbackAndValidation(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := loadServerClaimPrivateKey("", base64.RawStdEncoding.EncodeToString(privateKey))
	if err != nil || string(got) != string(privateKey) {
		t.Fatalf("fallback key load failed: %v", err)
	}
	if _, err := loadServerClaimPrivateKey("", "bad"); err == nil {
		t.Fatal("invalid key was accepted")
	}
}
