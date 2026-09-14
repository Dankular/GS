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
type LeaderboardRecord struct {
	UserID          string
	Score, Subscore int64
	Metadata        string
}

type TournamentConfig struct {
	ID               string
	DurationSeconds  int64
	ResetSchedule    string
	JoinRequired     bool
	MaxScoreAttempts int64
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
	if strings.TrimSpace(c.BaseURL) == "" || strings.TrimSpace(c.RuntimeHTTPKey) == "" || config.ID == "" || config.DurationSeconds <= 0 || record.UserID == "" {
		return errors.New("nakama tournament client is not configured")
	}
	payload := map[string]any{
		"tournamentId": config.ID, "ownerId": record.UserID, "username": "",
		"score": record.Score, "subscore": record.Subscore, "metadata": record.Metadata,
		"durationSeconds": config.DurationSeconds, "resetSchedule": config.ResetSchedule,
		"joinRequired": config.JoinRequired, "maxScoreAttempts": config.MaxScoreAttempts,
	}
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
