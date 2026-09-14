package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"time"

	"github.com/Dankular/GameService/internal/commandstore"
	"github.com/Dankular/GameService/internal/outbox"
)

func main() {
	ctx := context.Background()
	repo, err := commandstore.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	defer repo.Close()
	worker := outbox.Worker{Pool: repo.Pool(), Consumer: env("OUTBOX_CONSUMER", "stdout"), Publisher: outbox.JSONPublisher{Encoder: json.NewEncoder(os.Stdout)}}
	interval := 2 * time.Second
	for {
		if _, err := worker.RunOnce(ctx); err != nil {
			slog.Error("outbox cycle failed", "error", err)
		}
		time.Sleep(interval)
	}
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
