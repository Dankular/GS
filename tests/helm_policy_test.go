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
