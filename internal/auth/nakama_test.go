package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testToken(t *testing.T, claims map[string]any, secret string) string {
	t.Helper()
	header := rawURL.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	payload := rawURL.EncodeToString(payloadBytes)
	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write([]byte(header + "." + payload))
	return strings.Join([]string{header, payload, rawURL.EncodeToString(h.Sum(nil))}, ".")
}

func TestVerifyNakamaSession(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	token := testToken(t, map[string]any{"uid": "user-1", "iss": "nakama", "aud": []string{"gameservice"}, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(), "token_type": "session"}, "secret")
	claims, err := VerifyNakamaSession(token, "secret", "nakama", "gameservice", now)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != "user-1" {
		t.Fatalf("unexpected user: %s", claims.UserID)
	}
	if _, err := VerifyNakamaSession(token, "wrong", "nakama", "gameservice", now); err == nil {
		t.Fatal("expected signature rejection")
	}
	if _, err := VerifyNakamaSession(token, "secret", "nakama", "gameservice", now.Add(2*time.Hour)); err == nil {
		t.Fatal("expected expiry rejection")
	}
}

func TestVerifyNakamaSessionRejectsMalformedAndWrongContext(t *testing.T) {
	if _, err := VerifyNakamaSession("not-a-token", "secret", "", "", time.Now()); err == nil {
		t.Fatal("expected malformed rejection")
	}
	now := time.Unix(1_700_000_000, 0)
	token := testToken(t, map[string]any{"sub": "user-1", "iss": "other", "exp": now.Add(time.Hour).Unix()}, "secret")
	if _, err := VerifyNakamaSession(token, "secret", "nakama", "", now); err == nil {
		t.Fatal("expected issuer rejection")
	}
}

func TestSessionScopes(t *testing.T) {
	claims := SessionClaims{Scope: "definition:validate player:read"}
	if !claims.HasScope("definition:validate") || claims.HasScope("definition:publish") {
		t.Fatal("scope matching failed")
	}
	if !(SessionClaims{Vars: map[string]string{"scope": "definition:activate"}}).HasScope("definition:activate") {
		t.Fatal("vars scope matching failed")
	}
}

var _ = base64.RawURLEncoding
