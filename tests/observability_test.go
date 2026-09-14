package tests

import (
	"encoding/json"
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

func TestObservabilityDashboardUsesOnlyBoundedMetrics(t *testing.T) {
	data, err := os.ReadFile("../deploy/observability/gameservice-dashboard.json")
	if err != nil {
		data, err = os.ReadFile("deploy/observability/gameservice-dashboard.json")
	}
	if err != nil {
		t.Fatal(err)
	}
	var dashboard struct {
		Panels []struct {
			Title   string `json:"title"`
			Targets []struct {
				Expr string `json:"expr"`
			} `json:"targets"`
		} `json:"panels"`
	}
	if err := json.Unmarshal(data, &dashboard); err != nil {
		t.Fatalf("parse dashboard: %v", err)
	}
	if len(dashboard.Panels) < 4 {
		t.Fatalf("expected availability, traffic, error, and uptime panels")
	}
	for _, panel := range dashboard.Panels {
		for _, target := range panel.Targets {
			if strings.Contains(target.Expr, "player") || strings.Contains(target.Expr, "match") || strings.Contains(target.Expr, "request_id") {
				t.Fatalf("dashboard panel %q contains an unbounded identifier: %q", panel.Title, target.Expr)
			}
		}
	}
}
