package tests

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPrometheusRulesHaveRunbooks(t *testing.T) {
	data, err := os.ReadFile("../deploy/observability/prometheus-rules.yaml")
	if err != nil {
		// Tests in this package run from the repository root under `go test ./...`.
		data, err = os.ReadFile("deploy/observability/prometheus-rules.yaml")
	}
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Groups []struct {
			Rules []struct {
				Alert       string            `yaml:"alert"`
				Annotations map[string]string `yaml:"annotations"`
			} `yaml:"rules"`
		} `yaml:"groups"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatalf("parse Prometheus rules: %v", err)
	}
	if len(document.Groups) != 1 || len(document.Groups[0].Rules) < 3 {
		t.Fatalf("expected baseline availability, error-rate, and traffic alerts: %#v", document)
	}
	for _, rule := range document.Groups[0].Rules {
		if strings.TrimSpace(rule.Alert) == "" || strings.TrimSpace(rule.Annotations["runbook"]) == "" {
			t.Fatalf("alert %q is missing a runbook", rule.Alert)
		}
	}
}
