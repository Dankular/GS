//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"
)

func TestNakamaAuthoritativeTournamentWriteIsIdempotent(t *testing.T) {
	baseURL := os.Getenv("GAMESERVICE_INTEGRATION_NAKAMA_URL")
	httpKey := os.Getenv("GAMESERVICE_INTEGRATION_NAKAMA_RUNTIME_HTTP_KEY")
	tournamentID := os.Getenv("GAMESERVICE_INTEGRATION_TOURNAMENT_ID")
	ownerID := os.Getenv("GAMESERVICE_INTEGRATION_OWNER_ID")
	if baseURL == "" || httpKey == "" || tournamentID == "" || ownerID == "" {
		t.Skip("set GAMESERVICE_INTEGRATION_NAKAMA_URL, GAMESERVICE_INTEGRATION_NAKAMA_RUNTIME_HTTP_KEY, GAMESERVICE_INTEGRATION_TOURNAMENT_ID, and GAMESERVICE_INTEGRATION_OWNER_ID")
	}
	payload := map[string]any{
		"tournamentId": tournamentID, "eventKey": "integration-event-1", "ownerId": ownerID,
		"username": "integration", "score": 7, "subscore": 1,
		"metadata": map[string]any{"source": "integration"}, "durationSeconds": 3600,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	first := postRPC(t, ctx, baseURL, httpKey, payload)
	if duplicate, _ := first["duplicate"].(bool); duplicate {
		t.Fatalf("first tournament write was unexpectedly duplicate: %#v", first)
	}
	second := postRPC(t, ctx, baseURL, httpKey, payload)
	if duplicate, _ := second["duplicate"].(bool); !duplicate {
		t.Fatalf("second tournament write was not deduplicated: %#v", second)
	}
}

func postRPC(t *testing.T, ctx context.Context, baseURL, key string, payload map[string]any) map[string]any {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := fmt.Sprintf("%s/v2/rpc/gameservice.tournament_record?http_key=%s&unwrap", baseURL, url.QueryEscape(key))
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("Nakama runtime RPC returned %d: %s", response.StatusCode, data)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("decode runtime response: %v (%s)", err, data)
	}
	return result
}
