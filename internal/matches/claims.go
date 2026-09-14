package matches

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type JoinClaim struct {
	Issuer       string `json:"iss"`
	Audience     string `json:"aud"`
	Subject      string `json:"sub"`
	MatchID      string `json:"matchId"`
	AllocationID string `json:"allocationId"`
	ServerBuild  string `json:"serverBuild"`
	Team         string `json:"team,omitempty"`
	Slot         int    `json:"slot"`
	IssuedAt     int64  `json:"iat"`
	NotBefore    int64  `json:"nbf"`
	ExpiresAt    int64  `json:"exp"`
	JTI          string `json:"jti"`
}

var b64 = base64.RawURLEncoding

func NewKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

func SignClaim(claim JoinClaim, key ed25519.PrivateKey) (string, error) {
	if len(key) != ed25519.PrivateKeySize {
		return "", errors.New("invalid signing key")
	}
	if claim.Issuer == "" || claim.Audience == "" || claim.Subject == "" || claim.MatchID == "" || claim.AllocationID == "" || claim.ServerBuild == "" || claim.JTI == "" {
		return "", errors.New("claim required field missing")
	}
	header := b64.EncodeToString([]byte(`{"alg":"EdDSA","typ":"JWT"}`))
	payload, err := json.Marshal(claim)
	if err != nil {
		return "", err
	}
	encoded := header + "." + b64.EncodeToString(payload)
	return encoded + "." + b64.EncodeToString(ed25519.Sign(key, []byte(encoded))), nil
}

func VerifyClaim(token string, key ed25519.PublicKey, now time.Time, expectedAudience, expectedMatch, expectedBuild string) (JoinClaim, error) {
	if len(key) != ed25519.PublicKeySize {
		return JoinClaim{}, errors.New("invalid verification key")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return JoinClaim{}, errors.New("malformed claim")
	}
	header, err := b64.DecodeString(parts[0])
	if err != nil || string(header) != `{"alg":"EdDSA","typ":"JWT"}` {
		return JoinClaim{}, errors.New("invalid claim header")
	}
	payload, err := b64.DecodeString(parts[1])
	if err != nil {
		return JoinClaim{}, errors.New("invalid claim payload")
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil || !ed25519.Verify(key, []byte(parts[0]+"."+parts[1]), sig) {
		return JoinClaim{}, errors.New("invalid claim signature")
	}
	var c JoinClaim
	if err := json.Unmarshal(payload, &c); err != nil {
		return JoinClaim{}, err
	}
	sec := now.Unix()
	if c.Audience != expectedAudience || c.MatchID != expectedMatch || c.ServerBuild != expectedBuild {
		return JoinClaim{}, errors.New("claim context mismatch")
	}
	if c.NotBefore > sec || c.ExpiresAt <= sec {
		return JoinClaim{}, errors.New("claim is outside validity window")
	}
	return c, nil
}

func VerifyServerClaim(token string, key ed25519.PublicKey, now time.Time, expectedMatch, expectedAllocation, expectedBuild string) (JoinClaim, error) {
	claim, err := VerifyClaim(token, key, now, "control-plane", expectedMatch, expectedBuild)
	if err != nil {
		return JoinClaim{}, err
	}
	if claim.Subject != "game-server" || claim.AllocationID != expectedAllocation {
		return JoinClaim{}, errors.New("server claim allocation mismatch")
	}
	return claim, nil
}

func Digest(data []byte) string { h := sha256.Sum256(data); return fmt.Sprintf("sha256:%x", h[:]) }
