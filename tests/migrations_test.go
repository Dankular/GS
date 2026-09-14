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
		if !strings.Contains(text, "set -eu; for migration in /migrations/*.sql") || !strings.Contains(text, "*.down.sql") || !strings.Contains(text, "ON_ERROR_STOP=1") {
			t.Errorf("%s migration runner does not apply ordered, fail-fast migrations", name)
		}
	}
	configMap, err := os.ReadFile(filepath.Join("..", "deploy", "helm", "platform", "templates", "migration-configmap.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configMap), "005_definition_approvals.sql") || !strings.Contains(string(configMap), "files/005_definition_approvals.sql") {
		t.Fatal("Helm migration ConfigMap does not include the definition approvals migration")
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

func TestProductionHelmIncludesAgonesFleetWhenEnabled(t *testing.T) {
	values, err := os.ReadFile(filepath.Join("..", "deploy", "helm", "platform", "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	fleet, err := os.ReadFile(filepath.Join("..", "deploy", "helm", "platform", "templates", "gameserver-fleet.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"gameServer:", "enabled: false", "image: { repository: ghcr.io/dankular/gameservice-server, digest: \"\" }"} {
		if !strings.Contains(string(values), required) {
			t.Errorf("Helm values missing %q", required)
		}
	}
	for _, required := range []string{"agones.dev/v1", "FleetAutoscaler", "agones-sdk", "automountServiceAccountToken: false", "SERVER_CLAIM_PUBLIC_KEY"} {
		if !strings.Contains(string(fleet), required) {
			t.Errorf("Agones Fleet template missing %q", required)
		}
	}
	helpers, err := os.ReadFile(filepath.Join("..", "deploy", "helm", "platform", "templates", "_helpers.tpl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"regexMatch \"^sha256:[0-9a-f]{64}$\"", "fail (printf \"images.%s.digest must be an immutable sha256 digest\"", "gameservice.explicitImage"} {
		if !strings.Contains(string(helpers), required) {
			t.Errorf("Helm image helper missing immutable digest enforcement %q", required)
		}
	}
	policies, err := os.ReadFile(filepath.Join("..", "deploy", "helm", "platform", "templates", "gameserver-policies.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"gameservice-gameserver-default-deny", "gameservice-gameserver-ingress", "gameservice-gameserver-egress", "protocol: UDP", "port: 53"} {
		if !strings.Contains(string(policies), required) {
			t.Errorf("gameserver network policy missing %q", required)
		}
	}
	monitor, err := os.ReadFile(filepath.Join("..", "deploy", "helm", "platform", "templates", "service-monitor.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"monitoring.coreos.com/v1", "ServiceMonitor", "path: /metrics", "namespaceSelector", "enabled: false"} {
		if required == "enabled: false" {
			if !strings.Contains(string(values), "enabled: false") {
				t.Errorf("Helm metrics are not opt-in by default")
			}
			continue
		}
		if !strings.Contains(string(monitor), required) {
			t.Errorf("ServiceMonitor template missing %q", required)
		}
	}
	externalSecret, err := os.ReadFile(filepath.Join("..", "deploy", "helm", "platform", "templates", "external-secret.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"external-secrets.io/v1", "ExternalSecret", "secretStoreRef:", "dataFrom:", "extract:"} {
		if !strings.Contains(string(externalSecret), required) {
			t.Errorf("ExternalSecret template missing %q", required)
		}
	}
	if !strings.Contains(string(values), "externalSecrets:\r\n  enabled: false") && !strings.Contains(string(values), "externalSecrets:\n  enabled: false") {
		t.Error("external secret integration is not opt-in by default")
	}
}

func TestComposeSeparatesNakamaDatabaseRole(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "deploy", "compose", "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{"POSTGRES_USER: gameservice_admin", "database-admin-bootstrap:", "CREATE ROLE gameservice LOGIN PASSWORD %L NOSUPERUSER", "GRANT CONNECT, CREATE, TEMPORARY ON DATABASE gameservice TO gameservice", "nakama-role-bootstrap:", "CREATE ROLE nakama", "ALTER TABLE public.%I OWNER TO nakama", "ALTER SEQUENCE public.%I OWNER TO nakama", "ALTER ROLE gameservice NOSUPERUSER NOCREATEDB NOCREATEROLE", "\\gexec", "REVOKE ALL ON SCHEMA platform, economy, progression, match, ops FROM nakama", "database.address nakama:${NAKAMA_DATABASE_PASSWORD}@postgres:5432/gameservice", "POSTGRES_PASSWORD: ${GAMESERVICE_ADMIN_PASSWORD:?set GAMESERVICE_ADMIN_PASSWORD}", "NAKAMA_DATABASE_PASSWORD: ${NAKAMA_DATABASE_PASSWORD:?set NAKAMA_DATABASE_PASSWORD}"} {
		if !strings.Contains(text, required) {
			t.Errorf("Compose database boundary missing %q", required)
		}
	}
}
