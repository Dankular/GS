package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/Dankular/GameService/internal/commands"
	"github.com/Dankular/GameService/internal/commandstore"
	"github.com/Dankular/GameService/internal/economy"
	"github.com/jackc/pgx/v5"
)

func main() {
	addr := os.Getenv("CONTROL_API_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	repository, err := commandstore.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	defer repository.Close()
	economyService := economy.Service{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		if err := repository.Ping(r.Context()); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /v1/commands/{requestId}", func(w http.ResponseWriter, r *http.Request) {
		result, err := repository.Get(r.Context(), r.PathValue("requestId"))
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "command not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "command could not be loaded", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, result)
	})
	mux.HandleFunc("POST /v1/commands", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20+1))
		if err != nil {
			http.Error(w, "invalid request body", 400)
			return
		}
		if len(body) > 1<<20 {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		e, err := commands.DecodeStrict(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result, duplicate, err := repository.SubmitWith(r.Context(), e, economyService.Handle)
		if err != nil {
			slog.Error("command submission failed", "requestId", e.Metadata.RequestID, "correlationId", e.Metadata.CorrelationID, "error", err)
			http.Error(w, "command could not be stored", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if duplicate {
			w.Header().Set("X-Idempotency-Replay", "true")
		}
		writeJSON(w, result)
	})
	slog.Info("control API listening", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
