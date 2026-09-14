package nakama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func CallRuntimeRPC(ctx context.Context, httpClient *http.Client, baseURL, sessionToken, rpcID string, payload any) (map[string]any, error) {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(sessionToken) == "" || strings.TrimSpace(rpcID) == "" {
		return nil, errors.New("Nakama RPC configuration is incomplete")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode Nakama RPC payload: %w", err)
	}
	body, err := json.Marshal(string(payloadJSON))
	if err != nil {
		return nil, fmt.Errorf("encode Nakama RPC envelope: %w", err)
	}
	endpoint, err := url.JoinPath(strings.TrimRight(baseURL, "/"), "v2", "rpc", rpcID)
	if err != nil {
		return nil, fmt.Errorf("build Nakama RPC URL: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+sessionToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call Nakama RPC: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Nakama RPC returned HTTP %d", response.StatusCode)
	}
	var envelope struct {
		Payload string `json:"payload"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode Nakama RPC response: %w", err)
	}
	if envelope.Payload == "" {
		return map[string]any{}, nil
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(envelope.Payload), &result); err != nil {
		return nil, fmt.Errorf("decode Nakama RPC payload: %w", err)
	}
	return result, nil
}
