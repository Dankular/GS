//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Dankular/GameService/internal/matches"
)

type testConfig struct {
	APIURL                    string
	SigningKey                string
	SigningKeyFile            string
	PlayerA                   string
	PlayerB                   string
	GameID                    string
	Environment               string
	ModeID                    string
	Build                     string
	Region                    string
	ServerURL                 string
	ServerToken               string
	ServerTokenFile           string
	ServerClaimPrivateKeyFile string
}

type ticket struct {
	TicketID string `json:"ticketId"`
	MatchID  string `json:"matchId"`
	Status   string `json:"status"`
}

type match struct {
	MatchID      string         `json:"matchId"`
	State        string         `json:"state"`
	AllocationID string         `json:"allocationId"`
	ServerAddr   string         `json:"serverAddress"`
	ServerPorts  map[string]int `json:"serverPorts"`
	Build        string         `json:"build"`
	Roster       []struct {
		PlayerID string `json:"playerId"`
	} `json:"roster"`
}

func TestSyntheticMatchLifecycle(t *testing.T) {
	cfg, ok := loadConfig()
	if !ok {
		t.Skip("set GAMESERVICE_E2E_API_URL, a session signing key or GAMESERVICE_E2E_SESSION_SIGNING_KEY_FILE, GAMESERVICE_E2E_PLAYER_A, and GAMESERVICE_E2E_PLAYER_B to run the deployed Agones E2E")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client := &http.Client{Timeout: 10 * time.Second}
	tokenA := sessionToken(cfg.SigningKey, cfg.PlayerA)
	tokenB := sessionToken(cfg.SigningKey, cfg.PlayerB)

	expires := time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339)
	a := createTicket(t, ctx, client, cfg, tokenA, cfg.PlayerA, expires)
	b := createTicket(t, ctx, client, cfg, tokenB, cfg.PlayerB, expires)

	var current match
	poll(t, ctx, 2*time.Second, func() bool {
		var updated ticket
		getJSON(t, ctx, client, cfg.APIURL+"/v1/matchmaking/tickets/"+a.TicketID, tokenA, &updated, http.StatusOK)
		if updated.MatchID == "" {
			return false
		}
		a.MatchID = updated.MatchID
		getJSON(t, ctx, client, cfg.APIURL+"/v1/matchmaking/tickets/"+b.TicketID, tokenB, &updated, http.StatusOK)
		if updated.MatchID != a.MatchID {
			return false
		}
		getJSON(t, ctx, client, cfg.APIURL+"/v1/matches/"+a.MatchID, tokenA, &current, http.StatusOK)
		return current.State == "Ready" || current.State == "Running"
	})

	serverURL := cfg.ServerURL
	if serverURL == "" {
		port := current.ServerPorts["http"]
		if current.ServerAddr == "" || port == 0 {
			t.Fatal("match did not expose a simulator HTTP endpoint; set GAMESERVICE_E2E_SERVER_URL to a port-forward")
		}
		serverURL = "http://" + net.JoinHostPort(current.ServerAddr, strconv.Itoa(port))
	}
	join(t, ctx, client, serverURL, tokenForPlayer(t, ctx, client, cfg, tokenA, a.MatchID, cfg.PlayerA))
	join(t, ctx, client, serverURL, tokenForPlayer(t, ctx, client, cfg, tokenB, a.MatchID, cfg.PlayerB))

	payload := json.RawMessage(fmt.Sprintf(`{"players":[{"playerId":%q,"score":10,"subscore":0},{"playerId":%q,"score":5,"subscore":0}]}`, cfg.PlayerA, cfg.PlayerB))
	postJSON(t, ctx, client, serverURL+"/result", "", map[string]any{"sequence": 1, "payload": json.RawMessage(payload)})

	poll(t, ctx, 2*time.Second, func() bool {
		getJSON(t, ctx, client, cfg.APIURL+"/v1/matches/"+a.MatchID, tokenA, &current, http.StatusOK)
		return current.State == "Completed"
	})
	var completed ticket
	getJSON(t, ctx, client, cfg.APIURL+"/v1/matchmaking/tickets/"+a.TicketID, tokenA, &completed, http.StatusOK)
	if completed.Status != "matched" || completed.MatchID != a.MatchID {
		t.Fatalf("ticket was not durably associated with completed match: %+v", completed)
	}

	serverToken := cfg.ServerToken
	if serverToken == "" && cfg.ServerTokenFile != "" {
		data, err := os.ReadFile(cfg.ServerTokenFile)
		if err != nil {
			t.Fatalf("server token file could not be read: %v", err)
		}
		serverToken = strings.TrimSpace(string(data))
		if serverToken == "" {
			t.Fatal("server token file was empty")
		}
	}
	if serverToken == "" && cfg.ServerClaimPrivateKeyFile != "" {
		encodedKey, err := os.ReadFile(cfg.ServerClaimPrivateKeyFile)
		if err != nil {
			t.Fatalf("server claim private key file could not be read: %v", err)
		}
		key, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(string(encodedKey)))
		if err != nil || len(key) != ed25519.PrivateKeySize {
			t.Fatal("GAMESERVICE_E2E_SERVER_CLAIM_PRIVATE_KEY_FILE does not contain a valid Ed25519 private key")
		}
		now := time.Now().UTC()
		serverToken, err = matches.SignClaim(matches.JoinClaim{
			Issuer: "control-plane", Audience: "control-plane", Subject: "game-server",
			MatchID: current.MatchID, AllocationID: current.AllocationID, ServerBuild: current.Build,
			IssuedAt: now.Unix(), NotBefore: now.Unix(), ExpiresAt: now.Add(10 * time.Minute).Unix(),
			JTI: current.MatchID + ":" + current.AllocationID,
		}, ed25519.PrivateKey(key))
		if err != nil {
			t.Fatalf("server claim could not be signed: %v", err)
		}
	}
	if serverToken != "" {
		// The optional token allows the harness to verify duplicate result
		// acceptance through the authoritative server endpoint as well.
		result := map[string]any{"sequence": 1, "payload": json.RawMessage(payload)}
		body := postJSON(t, ctx, client, cfg.APIURL+"/v1/server/matches/"+a.MatchID+"/results", serverToken, result)
		if duplicate, _ := body["duplicate"].(bool); !duplicate {
			t.Fatalf("duplicate result was not acknowledged: %v", body)
		}
	}
}

func loadConfig() (testConfig, bool) {
	c := testConfig{
		APIURL: os.Getenv("GAMESERVICE_E2E_API_URL"), SigningKey: os.Getenv("GAMESERVICE_E2E_SESSION_SIGNING_KEY"), SigningKeyFile: os.Getenv("GAMESERVICE_E2E_SESSION_SIGNING_KEY_FILE"),
		PlayerA: os.Getenv("GAMESERVICE_E2E_PLAYER_A"), PlayerB: os.Getenv("GAMESERVICE_E2E_PLAYER_B"),
		GameID: envOr("GAMESERVICE_E2E_GAME_ID", "arena"), Environment: envOr("GAMESERVICE_E2E_ENVIRONMENT", "dev"),
		ModeID: envOr("GAMESERVICE_E2E_MODE_ID", "deathmatch"), Build: envOr("GAMESERVICE_E2E_BUILD", "sha256:0000000000000000000000000000000000000000000000000000000000000000"),
		Region: envOr("GAMESERVICE_E2E_REGION", "eu-west"), ServerURL: os.Getenv("GAMESERVICE_E2E_SERVER_URL"), ServerToken: os.Getenv("GAMESERVICE_E2E_SERVER_TOKEN"), ServerTokenFile: os.Getenv("GAMESERVICE_E2E_SERVER_TOKEN_FILE"), ServerClaimPrivateKeyFile: os.Getenv("GAMESERVICE_E2E_SERVER_CLAIM_PRIVATE_KEY_FILE"),
	}
	if c.SigningKey == "" && c.SigningKeyFile != "" {
		data, err := os.ReadFile(c.SigningKeyFile)
		if err == nil {
			c.SigningKey = strings.TrimSpace(string(data))
		}
	}
	return c, c.APIURL != "" && c.SigningKey != "" && c.PlayerA != "" && c.PlayerB != "" && c.PlayerA != c.PlayerB
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
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

func createTicket(t *testing.T, ctx context.Context, client *http.Client, cfg testConfig, token, player, expires string) ticket {
	t.Helper()
	var result ticket
	postJSONInto(t, ctx, client, cfg.APIURL+"/v1/matchmaking/tickets", token, map[string]any{"gameId": cfg.GameID, "environment": cfg.Environment, "modeId": cfg.ModeID, "definitionRevision": 1, "build": cfg.Build, "region": cfg.Region, "capacity": 2, "playerIds": []string{player}, "properties": map[string]any{}, "expiresAt": expires}, http.StatusAccepted, &result)
	if result.TicketID == "" {
		t.Fatal("ticket response did not contain ticketId")
	}
	return result
}

func tokenForPlayer(t *testing.T, ctx context.Context, client *http.Client, cfg testConfig, playerToken, matchID, player string) string {
	t.Helper()
	var result struct {
		Token string `json:"token"`
	}
	postJSONInto(t, ctx, client, cfg.APIURL+"/v1/matches/"+matchID+"/join-claims", playerToken, nil, http.StatusOK, &result)
	if result.Token == "" {
		t.Fatalf("join claim for %s was empty", player)
	}
	return result.Token
}

func join(t *testing.T, ctx context.Context, client *http.Client, serverURL, claim string) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/join", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+claim)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		t.Fatalf("join status %d: %s", resp.StatusCode, body)
	}
}

func poll(t *testing.T, ctx context.Context, interval time.Duration, check func() bool) {
	t.Helper()
	for {
		if check() {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(interval):
		}
	}
}

func getJSON(t *testing.T, ctx context.Context, client *http.Client, endpoint, token string, out any, status int) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != status {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		t.Fatalf("GET %s status %d: %s", endpoint, resp.StatusCode, body)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatal(err)
	}
}

func postJSON(t *testing.T, ctx context.Context, client *http.Client, endpoint, token string, body any) map[string]any {
	t.Helper()
	var result map[string]any
	postJSONInto(t, ctx, client, endpoint, token, body, http.StatusOK, &result)
	return result
}

func postJSONInto(t *testing.T, ctx context.Context, client *http.Client, endpoint, token string, body any, status int, out any) {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != status {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		t.Fatalf("POST %s status %d: %s", endpoint, resp.StatusCode, body)
	}
	if out != nil && resp.ContentLength != 0 {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
}
