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
