//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNakamaPrivacyExportDeleteIsRetryable(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("GAMESERVICE_INTEGRATION_NAKAMA_URL"), "/")
	serverKey := os.Getenv("GAMESERVICE_INTEGRATION_NAKAMA_SERVER_KEY")
	runtimeKey := os.Getenv("GAMESERVICE_INTEGRATION_NAKAMA_RUNTIME_HTTP_KEY")
	if baseURL == "" || serverKey == "" || runtimeKey == "" {
		t.Skip("set GAMESERVICE_INTEGRATION_NAKAMA_URL, GAMESERVICE_INTEGRATION_NAKAMA_SERVER_KEY, and GAMESERVICE_INTEGRATION_NAKAMA_RUNTIME_HTTP_KEY")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	account := authenticateDisposableDevice(t, ctx, baseURL, serverKey, suffix)
	var tokenClaims struct {
		UID string `json:"uid"`
	}
	if err := decodeJWTClaims(account.Token, &tokenClaims); err != nil || tokenClaims.UID == "" {
		t.Fatalf("decode Nakama session: %v", err)
	}
	payload := map[string]any{"operation": "export", "userId": tokenClaims.UID}
	if status, _ := postServerRPC(t, ctx, baseURL, runtimeKey, "gameservice.privacy", payload); status != http.StatusOK {
		t.Fatalf("privacy export HTTP %d", status)
	}
	payload["operation"] = "delete"
	if status, _ := postServerRPC(t, ctx, baseURL, runtimeKey, "gameservice.privacy", payload); status != http.StatusOK {
		t.Fatalf("privacy delete HTTP %d", status)
	}
	if status, body := postServerRPC(t, ctx, baseURL, runtimeKey, "gameservice.privacy", payload); status != http.StatusOK || !bytes.Contains(body, []byte(`"deleted":true`)) {
		t.Fatalf("privacy delete retry HTTP %d: %s", status, body)
	}
}

type disposableSession struct {
	Token string `json:"token"`
}

func authenticateDisposableDevice(t *testing.T, ctx context.Context, baseURL, serverKey, suffix string) disposableSession {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"id": "gameservice-privacy-test-" + suffix})
	endpoint := baseURL + "/v2/account/authenticate/device?create=true&username=gameservice_privacy_" + suffix
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.SetBasicAuth(serverKey, "")
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("authenticate disposable device HTTP %d: %s", response.StatusCode, data)
	}
	var session disposableSession
	if err := json.Unmarshal(data, &session); err != nil {
		t.Fatal(err)
	}
	return session
}

func decodeJWTClaims(token string, target any) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fmt.Errorf("invalid JWT")
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func postServerRPC(t *testing.T, ctx context.Context, baseURL, runtimeKey, rpcID string, payload map[string]any) (int, []byte) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v2/rpc/"+rpcID+"?http_key="+url.QueryEscape(runtimeKey)+"&unwrap", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	return response.StatusCode, body
}
