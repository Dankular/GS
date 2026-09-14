package api_test

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPIContainsImplementedSurface(t *testing.T) {
	data, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		OpenAPI string                         `yaml:"openapi"`
		Paths   map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatalf("OpenAPI is not valid YAML: %v", err)
	}
	if document.OpenAPI != "3.1.0" {
		t.Fatalf("unexpected OpenAPI version: %s", document.OpenAPI)
	}
	required := map[string]string{
		"/v1/commands": "post", "/v1/commands/{requestId}": "get",
		"/v1/players/me/snapshot": "get", "/v1/players/me/inventory": "get", "/v1/players/me/wallets": "get",
		"/v1/matchmaking/tickets": "post", "/v1/matchmaking/tickets/{ticketId}": "get",
		"/v1/matches/{matchId}": "get", "/v1/matches/{matchId}/join-claims": "post",
		"/v1/server/matches/{matchId}/ready": "post", "/v1/server/matches/{matchId}/heartbeat": "post", "/v1/server/matches/{matchId}/results": "post",
		"/v1/admin/definitions/validate": "post", "/v1/admin/definitions": "post",
		"/v1/admin/definitions/{revision}/activate": "post", "/v1/admin/definitions/{revision}/rollback": "post", "/v1/admin/audit": "get",
		"/health/live": "get", "/health/ready": "get", "/metrics": "get",
	}
	for path, method := range required {
		if _, ok := document.Paths[path][method]; !ok {
			t.Errorf("missing OpenAPI operation %s %s", method, path)
		}
	}
}
