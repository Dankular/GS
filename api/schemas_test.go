package api_test

import (
	"encoding/json"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"testing"
)

func TestVersionedSchemasAreValidJSON(t *testing.T) {
	files := []string{"command-envelope.schema.json", "game-definition.schema.json", "catalog.schema.json", "match-mode.schema.json", "reward-table.schema.json", "event-envelope.schema.json", "match-result-accepted.schema.json"}
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join("schemas", name))
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]any
		if err := json.Unmarshal(data, &document); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if document["$schema"] == nil || document["$id"] == nil {
			t.Fatalf("%s lacks schema identity", name)
		}
	}
}

func TestAsyncAPIContainsResultEvents(t *testing.T) {
	data, err := os.ReadFile("asyncapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		AsyncAPI string         `yaml:"asyncapi"`
		Channels map[string]any `yaml:"channels"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document.AsyncAPI != "3.0.0" || document.Channels["matchResultAccepted"] == nil || document.Channels["leaderboardDelivery"] == nil {
		t.Fatalf("invalid AsyncAPI event contract: %+v", document)
	}
}
