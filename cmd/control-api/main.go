package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/Dankular/GameService/internal/commands"
	"github.com/Dankular/GameService/internal/idempotency"
)

func main() {
	addr := os.Getenv("CONTROL_API_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	store := idempotency.New[commands.Result]()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
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
		result, duplicate := store.GetOrPut(e.Metadata.RequestID, func() commands.Result {
			return commands.Result{RequestID: e.Metadata.RequestID, CorrelationID: e.Metadata.CorrelationID, Status: "succeeded", Operation: e.Spec.Operation, Events: []string{"command.accepted.v1"}, Result: map[string]any{"accepted": true}}
		})
		if duplicate {
			result.Events = append(result.Events, "idempotency.hit.v1")
		}
		w.Header().Set("Content-Type", "application/json")
		if duplicate {
			w.Header().Set("X-Idempotency-Replay", "true")
		}
		json.NewEncoder(w).Encode(result)
	})
	slog.Info("control API listening", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
