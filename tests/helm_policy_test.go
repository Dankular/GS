package tests

import (
	"os"
	"strings"
	"testing"
)

func TestHelmPoliciesAllowInClusterPostgreSQL(t *testing.T) {
	data, err := os.ReadFile("../deploy/helm/platform/templates/policies.yaml")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, policy := range []string{"gameservice-control-egress", "gameservice-workers-egress"} {
		start := strings.Index(source, "name: "+policy)
		if start < 0 {
			t.Fatalf("missing policy %q", policy)
		}
		end := strings.Index(source[start:], "\n---")
		if end < 0 {
			end = len(source) - start
		}
		body := source[start : start+end]
		if !strings.Contains(body, "port: 5432") {
			t.Fatalf("policy %q does not allow PostgreSQL", policy)
		}
	}
}

func TestHelmMatchmakingWorkerLoadsClaimKeyFromSecretFile(t *testing.T) {
	data, err := os.ReadFile("../deploy/helm/platform/templates/matchmaking-worker.yaml")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, required := range []string{
		"SERVER_CLAIM_PRIVATE_KEY_FILE",
		"mountPath: /run/server-claims",
		"name: server-claims",
		"secretName: {{ .Values.serverClaims.privateKeySecretName }}",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("matchmaking worker secret-file wiring missing %q", required)
		}
	}
	if strings.Contains(source, "name: SERVER_CLAIM_PRIVATE_KEY, valueFrom") {
		t.Fatal("matchmaking worker still injects claim private key as an environment value")
	}
}

func TestComposeMatchmakingWorkerDoesNotInjectClaimPrivateKey(t *testing.T) {
	data, err := os.ReadFile("../deploy/compose/compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	if strings.Contains(source, "SERVER_CLAIM_PRIVATE_KEY: ${SERVER_CLAIM_PRIVATE_KEY") {
		t.Fatal("compose still injects the claim private key as an environment value")
	}
	for _, required := range []string{"SERVER_CLAIM_PRIVATE_KEY_FILE", "SERVER_CLAIM_PRIVATE_KEY_FILE_HOST", "/run/server-claims/private-key:ro"} {
		if !strings.Contains(source, required) {
			t.Fatalf("compose secret-file wiring missing %q", required)
		}
	}
}

func TestComposeControlAPIUsesJoinClaimKeyFile(t *testing.T) {
	data, err := os.ReadFile("../deploy/compose/compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	if strings.Contains(source, "JOIN_CLAIM_PRIVATE_KEY: ${JOIN_CLAIM_PRIVATE_KEY") {
		t.Fatal("compose still injects the join claim private key as an environment value")
	}
	for _, required := range []string{"JOIN_CLAIM_PRIVATE_KEY_FILE", "JOIN_CLAIM_PRIVATE_KEY_FILE_HOST", "/run/join-claims/private-key:ro"} {
		if !strings.Contains(source, required) {
			t.Fatalf("compose join-key wiring missing %q", required)
		}
	}
}

func TestHelmControlAPILoadsJoinClaimKeyFromSecretFile(t *testing.T) {
	data, err := os.ReadFile("../deploy/helm/platform/templates/control-api.yaml")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, required := range []string{"JOIN_CLAIM_PRIVATE_KEY_FILE", "mountPath: /run/server-claims", "secretName: {{ .Values.serverClaims.privateKeySecretName }}"} {
		if !strings.Contains(source, required) {
			t.Fatalf("control API secret-file wiring missing %q", required)
		}
	}
	if strings.Contains(source, "name: JOIN_CLAIM_PRIVATE_KEY\n") {
		t.Fatal("Helm still injects the join claim private key as an environment value")
	}
}
