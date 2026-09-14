package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeAndHelmMigrationsStayInSync(t *testing.T) {
	for _, name := range []string{"001_control.sql", "002_match_ticket_link.sql"} {
		compose, err := os.ReadFile(filepath.Join("..", "migrations", "control", name))
		if err != nil {
			t.Fatal(err)
		}
		helm, err := os.ReadFile(filepath.Join("..", "deploy", "helm", "platform", "files", name))
		if err != nil {
			t.Fatal(err)
		}
		if string(compose) != string(helm) {
			t.Fatalf("migration %s differs between Compose and Helm", name)
		}
	}
}

func TestDeploymentsApplyAllForwardMigrationsInOrder(t *testing.T) {
	compose, err := os.ReadFile(filepath.Join("..", "deploy", "compose", "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	helm, err := os.ReadFile(filepath.Join("..", "deploy", "helm", "platform", "templates", "migration-job.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"Compose": compose, "Helm": helm} {
		text := string(data)
		if !strings.Contains(text, "for migration in /migrations/*.sql") || !strings.Contains(text, "*.down.sql") || !strings.Contains(text, "ON_ERROR_STOP=1") {
			t.Errorf("%s migration runner does not apply ordered, fail-fast migrations", name)
		}
	}
}
