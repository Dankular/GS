package api_test

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPIContainsImplementedSurface(t *testing.T) {
	data, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		OpenAPI string                          `yaml:"openapi"`
		Paths   map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatalf("OpenAPI is not valid YAML: %v", err)
	}
	if document.OpenAPI != "3.0.3" {
		t.Fatalf("unexpected OpenAPI version: %s", document.OpenAPI)
	}
	required := map[string]string{
		"/v1/commands": "post", "/v1/commands/{requestId}": "get",
		"/v1/players/me/snapshot": "get", "/v1/players/me/inventory": "get", "/v1/players/me/wallets": "get",
		"/v1/matchmaking/tickets": "post", "/v1/matchmaking/tickets/{ticketId}": "get",
		"/v1/matches/{matchId}": "get", "/v1/matches/{matchId}/join-claims": "post",
		"/v1/server/matches/{matchId}/ready": "post", "/v1/server/matches/{matchId}/heartbeat": "post", "/v1/server/matches/{matchId}/results": "post",
		"/v1/admin/definitions/validate": "post", "/v1/admin/definitions/dry-run": "post", "/v1/admin/definitions": "post",
		"/v1/admin/definitions/{revision}/approval": "post", "/v1/admin/definitions/{revision}/activate": "post", "/v1/admin/definitions/{revision}/rollback": "post", "/v1/admin/audit": "get",
		"/health/live": "get", "/health/ready": "get", "/metrics": "get",
	}
	for path, method := range required {
		if _, ok := document.Paths[path][method]; !ok {
			t.Errorf("missing OpenAPI operation %s %s", method, path)
		}
	}
}

func TestOpenAPIDeclaresEveryPathParameter(t *testing.T) {
	data, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Paths map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	for path, node := range document.Paths {
		for _, segment := range strings.Split(path, "/") {
			if !strings.HasPrefix(segment, "{") || !strings.HasSuffix(segment, "}") {
				continue
			}
			name := strings.Trim(segment, "{}")
			if !pathHasParameter(node, name) {
				t.Errorf("path %q does not declare path parameter %q", path, name)
			}
		}
	}
}

func pathHasParameter(path yaml.Node, expected string) bool {
	if path.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(path.Content); i += 2 {
		if path.Content[i].Value != "parameters" || path.Content[i+1].Kind != yaml.SequenceNode {
			continue
		}
		for _, parameter := range path.Content[i+1].Content {
			if parameter.Kind != yaml.MappingNode {
				continue
			}
			name, in := "", ""
			for j := 0; j+1 < len(parameter.Content); j += 2 {
				switch parameter.Content[j].Value {
				case "name":
					name = parameter.Content[j+1].Value
				case "in":
					in = parameter.Content[j+1].Value
				}
			}
			if name == expected && in == "path" {
				return true
			}
		}
	}
	return false
}
