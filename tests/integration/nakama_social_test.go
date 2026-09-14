//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestNakamaSocialModerationRejectsConfiguredToken(t *testing.T) {
	baseURL := os.Getenv("GAMESERVICE_INTEGRATION_NAKAMA_URL")
	session := os.Getenv("GAMESERVICE_INTEGRATION_NAKAMA_SESSION")
	blocked := os.Getenv("GAMESERVICE_INTEGRATION_MODERATION_TOKEN")
	if baseURL == "" || session == "" || blocked == "" {
		t.Skip("set GAMESERVICE_INTEGRATION_NAKAMA_URL, GAMESERVICE_INTEGRATION_NAKAMA_SESSION, and GAMESERVICE_INTEGRATION_MODERATION_TOKEN")
	}
	payload := map[string]any{"operation": "chat.send", "channelId": "not-a-real-channel", "content": map[string]string{"text": blocked}}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(string(encoded))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/v2/rpc/gameservice.social", baseURL), bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+session)
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("moderated chat returned HTTP %d, want 403: %s", response.StatusCode, data)
	}
	if !bytes.Contains(data, []byte("chat message rejected by moderation policy")) {
		t.Fatalf("moderation response omitted policy error: %s", data)
	}
}
