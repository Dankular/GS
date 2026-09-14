package main

import (
	"context"
	"github.com/Dankular/GameService/internal/commandstore"
	"github.com/Dankular/GameService/internal/leaderboards"
	"github.com/Dankular/GameService/internal/nakama"
	"github.com/Dankular/GameService/internal/outbox"
	"log/slog"
	"net/http"
	"os"
	"time"
)

func main() {
	ctx := context.Background()
	repo, err := commandstore.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	defer repo.Close()
	worker := outbox.Worker{Pool: repo.Pool(), Consumer: "nakama-leaderboard", EventType: "match.result.accepted.v1", Publisher: leaderboards.Publisher{Pool: repo.Pool(), Nakama: nakama.Client{BaseURL: env("NAKAMA_URL", "http://nakama:7350"), ServerKey: os.Getenv("NAKAMA_SOCKET_SERVER_KEY"), RuntimeHTTPKey: os.Getenv("NAKAMA_RUNTIME_HTTP_KEY"), HTTP: &http.Client{Timeout: 5 * time.Second}}}}
	for {
		if _, err := worker.RunOnce(ctx); err != nil {
			slog.Error("leaderboard cycle failed", "error", err)
		}
		time.Sleep(2 * time.Second)
	}
}
func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
