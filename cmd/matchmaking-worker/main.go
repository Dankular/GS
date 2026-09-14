package main

import (
	"context"
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
	allocatorClient, err := allocation.NewGRPCAllocator(ctx, os.Getenv("AGONES_ALLOCATOR_ENDPOINT"), env("AGONES_ALLOCATOR_NAMESPACE", "default"), certPEM, keyPEM, caPEM)
	if err != nil {
		slog.Error("allocator initialization failed", "error", err)
		os.Exit(1)
	}
	defer allocatorClient.Close()
	worker := matchmaking.Worker{Pool: repo.Pool(), Allocator: allocatorClient, MatchStore: matches.Store{Pool: repo.Pool()}, Policy: matchmaking.Policy{TeamSize: envInt("MATCH_TEAM_SIZE", 1), Teams: envInt("MATCH_TEAMS", 2), RatingWindow: int64(envInt("MATCH_RATING_WINDOW", 0))}, Protocol: env("MATCH_PROTOCOL", "udp")}
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
