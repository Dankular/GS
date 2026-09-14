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
	BaseURL, ServerKey string
	HTTP               *http.Client
}
type LeaderboardRecord struct {
	UserID          string
	Score, Subscore int64
	Metadata        string
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
