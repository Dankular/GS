//go:build load

package load

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigReadsSigningKeyFromFile(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "session-key")
	if err := os.WriteFile(keyPath, []byte("file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GAMESERVICE_LOAD_API_URL", "http://127.0.0.1:8080")
	t.Setenv("GAMESERVICE_LOAD_SESSION_SIGNING_KEY_FILE", keyPath)
	t.Setenv("GAMESERVICE_LOAD_PLAYER", "load-test")
	t.Setenv("GAMESERVICE_LOAD_REQUESTS", "1")
	t.Setenv("GAMESERVICE_LOAD_WORKERS", "1")
	t.Setenv("GAMESERVICE_LOAD_SESSION_SIGNING_KEY", "")

	config, ok := loadConfig()
	if !ok || config.Secret != "file-secret" {
		t.Fatalf("protected signing key was not loaded: ok=%v secret=%q", ok, config.Secret)
	}
}
