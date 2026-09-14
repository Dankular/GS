package nakama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	BaseURL, ServerKey, RuntimeHTTPKey string
	HTTP                               *http.Client
}

// Health checks Nakama's supported unauthenticated health endpoint.
func (c Client) Health(ctx context.Context) error {
	if strings.TrimSpace(c.BaseURL) == "" {
		return errors.New("nakama health client is not configured")
	}
	endpoint, err := url.JoinPath(strings.TrimRight(c.BaseURL, "/"), "healthcheck")
	if err != nil {
		return fmt.Errorf("build Nakama health URL: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("check Nakama health: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Nakama health returned HTTP %d", response.StatusCode)
	}
	return nil
}

type LeaderboardRecord struct {
	UserID          string
	Score, Subscore int64
	Metadata        string
}

type TournamentConfig struct {
	ID               string
	EventKey         string
	DurationSeconds  int64
	ResetSchedule    string
	JoinRequired     bool
	MaxScoreAttempts int64
}

// RuntimeRPC invokes a server-only Nakama JavaScript runtime RPC. The caller
// must provide a runtime HTTP key; this boundary never uses client sessions.
func (c Client) RuntimeRPC(ctx context.Context, rpcID string, payload any, result any) error {
	if strings.TrimSpace(c.BaseURL) == "" || strings.TrimSpace(c.RuntimeHTTPKey) == "" || strings.TrimSpace(rpcID) == "" {
		return errors.New("nakama runtime client is not configured")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Nakama runtime payload: %w", err)
	}
	endpoint, err := url.JoinPath(strings.TrimRight(c.BaseURL, "/"), "v2", "rpc", rpcID)
	if err != nil {
		return fmt.Errorf("build Nakama runtime URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"?http_key="+url.QueryEscape(c.RuntimeHTTPKey)+"&unwrap", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call Nakama runtime RPC: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("Nakama runtime RPC returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if result == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decode Nakama runtime RPC response: %w", err)
	}
	return nil
}

func (c Client) WriteLeaderboardRecord(ctx context.Context, leaderboardID string, record LeaderboardRecord) error {
	if strings.TrimSpace(c.BaseURL) == "" || c.ServerKey == "" || leaderboardID == "" || record.UserID == "" {
		return errors.New("nakama leaderboard client is not configured")
	}
	body, err := json.Marshal(map[string]any{"record": map[string]any{"owner_id": record.UserID, "score": record.Score, "subscore": record.Subscore, "metadata": record.Metadata}})
	if err != nil {
		return err
	}
	endpoint, err := url.JoinPath(strings.TrimRight(c.BaseURL, "/"), "v2", "leaderboard", leaderboardID)
	if err != nil {
		return fmt.Errorf("build Nakama leaderboard URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(c.ServerKey, "")
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("write Nakama leaderboard record: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("Nakama leaderboard write returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// WriteTournamentRecord delegates authoritative tournament creation and record
// writes to Nakama's server-to-server runtime RPC. Nakama only exposes
// tournament creation and authoritative writes through the runtime API; the
// worker never accesses Nakama-owned tables.
func (c Client) WriteTournamentRecord(ctx context.Context, config TournamentConfig, record LeaderboardRecord) error {
	if strings.TrimSpace(c.BaseURL) == "" || strings.TrimSpace(c.RuntimeHTTPKey) == "" || config.ID == "" || config.EventKey == "" || config.DurationSeconds <= 0 || record.UserID == "" {
		return errors.New("nakama tournament client is not configured")
	}
	payload := map[string]any{
		"tournamentId": config.ID, "eventKey": config.EventKey, "ownerId": record.UserID, "username": "",
		"score": record.Score, "subscore": record.Subscore,
		"durationSeconds": config.DurationSeconds, "resetSchedule": config.ResetSchedule,
		"joinRequired": config.JoinRequired, "maxScoreAttempts": config.MaxScoreAttempts,
	}
	metadata := map[string]any{}
	if strings.TrimSpace(record.Metadata) != "" {
		if err := json.Unmarshal([]byte(record.Metadata), &metadata); err != nil {
			return fmt.Errorf("decode Nakama tournament metadata: %w", err)
		}
	}
	payload["metadata"] = metadata
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Nakama tournament record: %w", err)
	}
	endpoint, err := url.JoinPath(strings.TrimRight(c.BaseURL, "/"), "v2", "rpc", "gameservice.tournament_record")
	if err != nil {
		return fmt.Errorf("build Nakama tournament URL: %w", err)
	}
	query := endpoint + "?http_key=" + url.QueryEscape(c.RuntimeHTTPKey) + "&unwrap"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, query, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("write Nakama tournament record: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
		return fmt.Errorf("Nakama tournament write returned HTTP %d", response.StatusCode)
	}
	return nil
}
