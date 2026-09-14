package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeAndHelmMigrationsStayInSync(t *testing.T) {
	for _, name := range []string{"001_control.sql", "002_match_ticket_link.sql", "003_privacy.sql", "004_audit_chain.sql", "005_definition_approvals.sql"} {
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

func TestAuditMigrationIsAppendOnlyAndHasHashChain(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "migrations", "control", "004_audit_chain.sql"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{"record_hash", "previous_hash", "audit_log_immutable", "BEFORE UPDATE OR DELETE", "audit_log_hash_chain", "pg_advisory_xact_lock"} {
		if !strings.Contains(text, required) {
			t.Errorf("audit migration missing %q", required)
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
	if !strings.Contains(string(compose), "migrations:\n") || !strings.Contains(string(compose), "postgres: { condition: service_healthy }") {
		t.Fatal("Compose migrations do not wait for healthy PostgreSQL")
	}
}

func TestComposeSeedUsesDefinitionMountedInSeedImage(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "deploy", "compose", "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "--file\", \"/definitions/examples/arena.yaml") {
		t.Fatal("Compose seed job does not use the mounted definition path")
	}
}

func TestSupplyChainPolicyIsOptInAndKeyless(t *testing.T) {
	values, err := os.ReadFile(filepath.Join("..", "deploy", "helm", "platform", "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	policy, err := os.ReadFile(filepath.Join("..", "deploy", "helm", "platform", "templates", "image-signature-policy.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(values), "admissionPolicy:\n    enabled: false") {
		t.Fatal("image admission policy is not opt-in by default")
	}
	for _, required := range []string{"verifyImages:", "mutateDigest: false", "verifyDigest: true", "keyless:", "issuerRegExp:", "subjectRegExp:", "rekor:"} {
		if !strings.Contains(string(policy), required) {
			t.Errorf("image admission policy missing %q", required)
		}
	}
}

func TestComposeSeparatesNakamaDatabaseRole(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "deploy", "compose", "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{"nakama-role-bootstrap:", "CREATE ROLE nakama", "\\gexec", "REVOKE ALL ON SCHEMA platform, economy, progression, match, ops FROM nakama", "database.address nakama:${NAKAMA_DATABASE_PASSWORD}@postgres:5432/gameservice", "NAKAMA_DATABASE_PASSWORD: ${NAKAMA_DATABASE_PASSWORD:?set NAKAMA_DATABASE_PASSWORD}"} {
		if !strings.Contains(text, required) {
			t.Errorf("Compose database boundary missing %q", required)
		}
	}
}
