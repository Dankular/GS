package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type SessionClaims struct {
	UserID    string            `json:"uid"`
	Subject   string            `json:"sub"`
	Issuer    string            `json:"iss"`
	Audience  any               `json:"aud"`
	TokenType string            `json:"token_type"`
	Scope     string            `json:"scope"`
	Vars      map[string]string `json:"vars"`
	IssuedAt  int64             `json:"iat"`
	NotBefore int64             `json:"nbf"`
	ExpiresAt int64             `json:"exp"`
}

func (c SessionClaims) HasScope(required string) bool {
	if c.Scope == required {
		return true
	}
	for _, scope := range strings.Fields(c.Scope) {
		if scope == required {
			return true
		}
	}
	return c.Vars != nil && c.Vars["scope"] == required
}

var rawURL = base64.RawURLEncoding

func VerifyNakamaSession(token, secret, expectedIssuer, expectedAudience string, now time.Time) (SessionClaims, error) {
	if strings.TrimSpace(token) == "" || secret == "" {
		return SessionClaims{}, errors.New("missing session credentials")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return SessionClaims{}, errors.New("malformed session token")
	}
	headerBytes, err := rawURL.DecodeString(parts[0])
	if err != nil {
		return SessionClaims{}, errors.New("invalid session header")
	}
	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	if json.Unmarshal(headerBytes, &header) != nil || header.Algorithm != "HS256" || header.Type != "JWT" {
		return SessionClaims{}, errors.New("unsupported session token")
	}
	expected := hmac.New(sha256.New, []byte(secret))
	_, _ = expected.Write([]byte(parts[0] + "." + parts[1]))
	signature, err := rawURL.DecodeString(parts[2])
	if err != nil || !hmac.Equal(signature, expected.Sum(nil)) {
		return SessionClaims{}, errors.New("invalid session signature")
	}
	payload, err := rawURL.DecodeString(parts[1])
	if err != nil {
		return SessionClaims{}, errors.New("invalid session payload")
	}
	var claims SessionClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return SessionClaims{}, fmt.Errorf("decode session claims: %w", err)
	}
	if claims.UserID == "" {
		claims.UserID = claims.Subject
	}
	if claims.UserID == "" || claims.ExpiresAt <= 0 {
		return SessionClaims{}, errors.New("session identity or expiry missing")
	}
	sec := now.Unix()
	if claims.NotBefore > sec || claims.ExpiresAt <= sec {
		return SessionClaims{}, errors.New("session is outside validity window")
	}
	if expectedIssuer != "" && claims.Issuer != expectedIssuer {
		return SessionClaims{}, errors.New("session issuer mismatch")
	}
	if expectedAudience != "" && !audienceContains(claims.Audience, expectedAudience) {
		return SessionClaims{}, errors.New("session audience mismatch")
	}
	if claims.TokenType != "" && claims.TokenType != "session" {
		return SessionClaims{}, errors.New("invalid session token type")
	}
	return claims, nil
}

func audienceContains(value any, expected string) bool {
	switch v := value.(type) {
	case string:
		return v == expected
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == expected {
				return true
			}
		}
	}
	return false
}
