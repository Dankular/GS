package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Dankular/GameService/internal/allocation"
	"github.com/Dankular/GameService/internal/commandstore"
	"github.com/Dankular/GameService/internal/matches"
	"github.com/Dankular/GameService/internal/matchmaking"
)

func main() {
	ctx := context.Background()
	repo, err := commandstore.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	defer repo.Close()
	caPEM, err := os.ReadFile(os.Getenv("AGONES_ALLOCATOR_CA_FILE"))
	if err != nil {
		slog.Error("allocator CA could not be loaded", "error", err)
		os.Exit(1)
	}
	certPEM, err := os.ReadFile(os.Getenv("AGONES_ALLOCATOR_CERT_FILE"))
	if err != nil {
		slog.Error("allocator client certificate could not be loaded", "error", err)
		os.Exit(1)
	}
	keyPEM, err := os.ReadFile(os.Getenv("AGONES_ALLOCATOR_KEY_FILE"))
	if err != nil {
		slog.Error("allocator client key could not be loaded", "error", err)
		os.Exit(1)
	}
	allocatorClient, err := allocation.NewGRPCAllocatorWithServerName(ctx, os.Getenv("AGONES_ALLOCATOR_ENDPOINT"), env("AGONES_ALLOCATOR_NAMESPACE", "default"), certPEM, keyPEM, caPEM, os.Getenv("AGONES_ALLOCATOR_SERVER_NAME"))
	if err != nil {
		slog.Error("allocator initialization failed", "error", err)
		os.Exit(1)
	}
	defer allocatorClient.Close()
	worker := matchmaking.Worker{Pool: repo.Pool(), Allocator: allocatorClient, MatchStore: matches.Store{Pool: repo.Pool()}, Policy: matchmaking.Policy{TeamSize: envInt("MATCH_TEAM_SIZE", 1), Teams: envInt("MATCH_TEAMS", 2), RatingWindow: int64(envInt("MATCH_RATING_WINDOW", 0))}, Protocol: env("MATCH_PROTOCOL", "udp")}
	if key, keyErr := loadServerClaimPrivateKey(os.Getenv("SERVER_CLAIM_PRIVATE_KEY_FILE"), os.Getenv("SERVER_CLAIM_PRIVATE_KEY")); keyErr != nil {
		slog.Error("server claim private key could not be loaded", "error", keyErr)
		os.Exit(1)
	} else if key != nil {
		worker.ServerClaimPrivateKey = key
	}
	worker.ServerClaimTTL = time.Duration(envInt("SERVER_CLAIM_TTL_SECONDS", 600)) * time.Second
	interval := time.Duration(envInt("MATCHMAKING_INTERVAL_SECONDS", 2)) * time.Second
	for {
		if matched, err := worker.RunOnce(ctx); err != nil {
			slog.Error("matchmaking cycle failed", "error", err)
		} else if matched {
			slog.Info("match allocated")
		}
		time.Sleep(interval)
	}
}

func loadServerClaimPrivateKey(path, encoded string) (ed25519.PrivateKey, error) {
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		encoded = string(data)
	}
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, nil
	}
	key, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("server claim private key must be base64 Ed25519 private key")
	}
	return ed25519.PrivateKey(key), nil
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}
