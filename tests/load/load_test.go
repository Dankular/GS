//go:build load

package load

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type config struct {
	BaseURL  string
	Secret   string
	Player   string
	GameID   string
	Env      string
	Mode     string
	Build    string
	Region   string
	Workers  int
	Requests int
}

func TestHTTPProfiles(t *testing.T) {
	c, ok := loadConfig()
	if !ok {
		t.Skip("set GAMESERVICE_LOAD_API_URL, GAMESERVICE_LOAD_SESSION_SIGNING_KEY, and GAMESERVICE_LOAD_PLAYER to run the HTTP load profiles")
	}
	profiles := strings.Split(envOr("GAMESERVICE_LOAD_PROFILES", "auth,snapshot,inventory,matchmaking"), ",")
	for _, profile := range profiles {
		profile = strings.TrimSpace(profile)
		if profile == "" {
			continue
		}
		t.Run(profile, func(t *testing.T) { runProfile(t, c, profile) })
	}
}

func loadConfig() (config, bool) {
	c := config{
		BaseURL:  strings.TrimRight(os.Getenv("GAMESERVICE_LOAD_API_URL"), "/"),
		Secret:   os.Getenv("GAMESERVICE_LOAD_SESSION_SIGNING_KEY"),
		Player:   os.Getenv("GAMESERVICE_LOAD_PLAYER"),
		GameID:   envOr("GAMESERVICE_LOAD_GAME_ID", "arena"),
		Env:      envOr("GAMESERVICE_LOAD_ENVIRONMENT", "dev"),
		Mode:     envOr("GAMESERVICE_LOAD_MODE_ID", "deathmatch"),
		Build:    envOr("GAMESERVICE_LOAD_BUILD", "sha256:0000000000000000000000000000000000000000000000000000000000000000"),
		Region:   envOr("GAMESERVICE_LOAD_REGION", "eu-west"),
		Workers:  envInt("GAMESERVICE_LOAD_WORKERS", 4),
		Requests: envInt("GAMESERVICE_LOAD_REQUESTS", 20),
	}
	return c, c.BaseURL != "" && c.Secret != "" && c.Player != "" && c.Workers > 0 && c.Requests > 0
}

func runProfile(t *testing.T, c config, profile string) {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	token := sessionToken(c.Secret, c.Player)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var mu sync.Mutex
	latencies := make([]time.Duration, 0, c.Requests)
	var failures []string
	jobs := make(chan int)
	var wg sync.WaitGroup
	for worker := 0; worker < c.Workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				started := time.Now()
				status, body, err := execute(ctx, client, c, token, profile)
				mu.Lock()
				latencies = append(latencies, time.Since(started))
				if err != nil {
					failures = append(failures, err.Error())
				} else if status < 200 || status >= 300 {
					failures = append(failures, fmt.Sprintf("status=%d body=%s", status, body))
				}
				mu.Unlock()
			}
		}()
	}
	for i := 0; i < c.Requests; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	if len(failures) > 0 {
		t.Fatalf("profile %s had %d/%d failures (first: %s)", profile, len(failures), c.Requests, failures[0])
	}
	sortDurations(latencies)
	t.Logf("profile=%s requests=%d workers=%d p50=%s p95=%s max=%s", profile, len(latencies), c.Workers, percentile(latencies, .50), percentile(latencies, .95), latencies[len(latencies)-1])
}

func execute(ctx context.Context, client *http.Client, c config, token, profile string) (int, string, error) {
	switch profile {
	case "auth", "snapshot":
		return request(ctx, client, http.MethodGet, c.BaseURL+"/v1/players/me/snapshot", token, nil)
	case "inventory":
		return request(ctx, client, http.MethodGet, c.BaseURL+"/v1/players/me/inventory", token, nil)
	case "matchmaking":
		expires := time.Now().UTC().Add(2 * time.Minute).Format(time.RFC3339)
		status, body, err := request(ctx, client, http.MethodPost, c.BaseURL+"/v1/matchmaking/tickets", token, map[string]any{
			"gameId": c.GameID, "environment": c.Env, "modeId": c.Mode, "definitionRevision": 1,
			"build": c.Build, "region": c.Region, "capacity": 2, "playerIds": []string{c.Player},
			"properties": map[string]any{}, "expiresAt": expires,
		})
		if err != nil || status < 200 || status >= 300 {
			return status, body, err
		}
		var ticket struct {
			TicketID string `json:"ticketId"`
		}
		if err := json.Unmarshal([]byte(body), &ticket); err != nil || ticket.TicketID == "" {
			return status, body, fmt.Errorf("matchmaking response has no ticketId")
		}
		deleteStatus, deleteBody, deleteErr := request(ctx, client, http.MethodDelete, c.BaseURL+"/v1/matchmaking/tickets/"+ticket.TicketID, token, nil)
		return deleteStatus, deleteBody, deleteErr
	default:
		return 0, "", fmt.Errorf("unsupported load profile %q", profile)
	}
}

func request(ctx context.Context, client *http.Client, method, endpoint, token string, payload any) (int, string, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return 0, "", err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, string(data), nil
}

func sessionToken(secret, player string) string {
	encode := func(value any) string {
		data, _ := json.Marshal(value)
		return base64.RawURLEncoding.EncodeToString(data)
	}
	header := encode(map[string]string{"alg": "HS256", "typ": "JWT"})
	now := time.Now().Unix()
	payload := encode(map[string]any{"uid": player, "sub": player, "token_type": "session", "scope": "player:read", "iat": now, "nbf": now - 1, "exp": now + 600})
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(header + "." + payload))
	return header + "." + payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func sortDurations(values []time.Duration) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func percentile(values []time.Duration, fraction float64) time.Duration {
	index := int(float64(len(values)-1) * fraction)
	return values[index]
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(envOr(name, strconv.Itoa(fallback)))
	if err != nil {
		return fallback
	}
	return value
}
