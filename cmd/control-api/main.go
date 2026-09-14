package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Dankular/GameService/internal/auth"
	"github.com/Dankular/GameService/internal/commands"
	"github.com/Dankular/GameService/internal/commandstore"
	"github.com/Dankular/GameService/internal/compiler"
	"github.com/Dankular/GameService/internal/definitions"
	"github.com/Dankular/GameService/internal/economy"
	"github.com/Dankular/GameService/internal/matches"
	"github.com/Dankular/GameService/internal/matchmaking"
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
	matchmakingStore := matchmaking.Store{Pool: repository.Pool()}
	definitionStore := definitions.Store{Pool: repository.Pool()}
	sessionSigningKey := os.Getenv("NAKAMA_SESSION_SIGNING_KEY")
	if sessionSigningKey == "" {
		slog.Error("NAKAMA_SESSION_SIGNING_KEY is required")
		os.Exit(1)
	}
	issuer := os.Getenv("NAKAMA_SESSION_ISSUER")
	audience := os.Getenv("NAKAMA_SESSION_AUDIENCE")
	serverPublicKey, _ := base64.RawStdEncoding.DecodeString(os.Getenv("SERVER_CLAIM_PUBLIC_KEY"))
	authenticate := func(r *http.Request) (auth.SessionClaims, error) {
		return auth.VerifyNakamaSession(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), sessionSigningKey, issuer, audience, time.Now())
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		if err := repository.Ping(r.Context()); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /v1/players/me/snapshot", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := requireScope(w, r, authenticate, "player:read")
		if !ok {
			return
		}
		snapshot, err := economy.Snapshot(r.Context(), repository.Pool(), claims.UserID)
		if err != nil {
			http.Error(w, "snapshot unavailable", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, snapshot)
	})
	mux.HandleFunc("GET /v1/players/me/inventory", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := requireScope(w, r, authenticate, "player:read")
		if !ok {
			return
		}
		items, err := economy.Inventory(r.Context(), repository.Pool(), claims.UserID)
		if err != nil {
			http.Error(w, "inventory unavailable", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, map[string]any{"playerId": claims.UserID, "items": items})
	})
	mux.HandleFunc("GET /v1/players/me/wallets", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := requireScope(w, r, authenticate, "player:read")
		if !ok {
			return
		}
		wallets, err := economy.Wallets(r.Context(), repository.Pool(), claims.UserID)
		if err != nil {
			http.Error(w, "wallets unavailable", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, map[string]any{"playerId": claims.UserID, "wallets": wallets})
	})
	mux.HandleFunc("GET /v1/commands/{requestId}", func(w http.ResponseWriter, r *http.Request) {
		if _, err := authenticate(r); err != nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
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
		claims, err := authenticate(r)
		if err != nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
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
		if e.Actor.ID != claims.UserID {
			http.Error(w, "actor does not match authenticated user", http.StatusForbidden)
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
	mux.HandleFunc("POST /v1/matchmaking/tickets", func(w http.ResponseWriter, r *http.Request) {
		claims, err := authenticate(r)
		if err != nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)
			return
		}
		var request matchmaking.TicketRequest
		decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			http.Error(w, "invalid ticket request", http.StatusBadRequest)
			return
		}
		ticket, err := matchmakingStore.Create(r.Context(), request, claims.UserID, time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		writeJSON(w, ticket)
	})
	mux.HandleFunc("GET /v1/matchmaking/tickets/{ticketId}", func(w http.ResponseWriter, r *http.Request) {
		claims, err := authenticate(r)
		if err != nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		ticket, err := matchmakingStore.Get(r.Context(), r.PathValue("ticketId"), claims.UserID)
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "ticket not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "ticket could not be loaded", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, ticket)
	})
	mux.HandleFunc("DELETE /v1/matchmaking/tickets/{ticketId}", func(w http.ResponseWriter, r *http.Request) {
		claims, err := authenticate(r)
		if err != nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		if err := matchmakingStore.Cancel(r.Context(), r.PathValue("ticketId"), claims.UserID); errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "ticket not found or not cancellable", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "ticket could not be cancelled", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /v1/server/matches/{matchId}/results", func(w http.ResponseWriter, r *http.Request) {
		if len(serverPublicKey) != ed25519.PublicKeySize {
			http.Error(w, "server claim verification is not configured", http.StatusServiceUnavailable)
			return
		}
		matchID := r.PathValue("matchId")
		var expectedBuild, expectedAllocation, state string
		if err := repository.Pool().QueryRow(r.Context(), `SELECT server_build,COALESCE(allocation_id,''),state FROM match.matches WHERE match_id=$1`, matchID).Scan(&expectedBuild, &expectedAllocation, &state); errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "match not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "match could not be loaded", http.StatusServiceUnavailable)
			return
		}
		if state != "Running" && state != "Finalizing" {
			http.Error(w, "match is not accepting results", http.StatusConflict)
			return
		}
		_, err := authenticateServer(r, ed25519.PublicKey(serverPublicKey), matchID, expectedAllocation, expectedBuild)
		if err != nil {
			http.Error(w, "invalid server claim", http.StatusUnauthorized)
			return
		}
		var request struct {
			Sequence      int64           `json:"sequence"`
			Payload       json.RawMessage `json:"payload"`
			PayloadDigest string          `json:"payloadDigest"`
		}
		decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			http.Error(w, "invalid result request", http.StatusBadRequest)
			return
		}
		tx, err := repository.Pool().Begin(r.Context())
		if err != nil {
			http.Error(w, "result transaction unavailable", http.StatusServiceUnavailable)
			return
		}
		defer tx.Rollback(r.Context())
		duplicate, digest, err := matches.SubmitResult(r.Context(), tx, matches.ResultSubmission{MatchID: matchID, Sequence: request.Sequence, Payload: request.Payload, PayloadDigest: request.PayloadDigest})
		if errors.Is(err, matches.ErrResultDigestMismatch) {
			http.Error(w, "result digest conflict", http.StatusConflict)
			return
		}
		if errors.Is(err, matches.ErrMatchNotRunning) {
			http.Error(w, "match is not running", http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, "result rejected", http.StatusUnprocessableEntity)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "result commit failed", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, map[string]any{"accepted": true, "duplicate": duplicate, "payloadDigest": digest})
	})
	mux.HandleFunc("POST /v1/admin/definitions/validate", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireScope(w, r, authenticate, "definition:validate"); !ok {
			return
		}
		source, err := readDefinitionSource(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		report, err := compiler.Compile(strings.NewReader(string(source)))
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		writeJSON(w, report)
	})
	mux.HandleFunc("POST /v1/admin/definitions", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := requireScope(w, r, authenticate, "definition:publish")
		if !ok {
			return
		}
		reason := strings.TrimSpace(r.Header.Get("X-Reason"))
		if reason == "" {
			http.Error(w, "X-Reason is required", http.StatusBadRequest)
			return
		}
		source, err := readDefinitionSource(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		report, err := compiler.Compile(strings.NewReader(string(source)))
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		if err := definitionStore.Publish(r.Context(), report, string(source), claims.UserID, reason, true); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]any{"gameId": report.Definition.Metadata.GameID, "revision": report.Definition.Metadata.Revision, "digest": report.Digest, "status": "published"})
	})
	mux.HandleFunc("POST /v1/admin/definitions/{revision}/activate", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := requireScope(w, r, authenticate, "definition:activate")
		if !ok {
			return
		}
		revision, err := strconv.ParseInt(r.PathValue("revision"), 10, 64)
		if err != nil {
			http.Error(w, "revision must be an integer", http.StatusBadRequest)
			return
		}
		gameID := strings.TrimSpace(r.URL.Query().Get("gameId"))
		environment := strings.TrimSpace(r.URL.Query().Get("environment"))
		reason := strings.TrimSpace(r.Header.Get("X-Reason"))
		if gameID == "" || environment == "" || reason == "" {
			http.Error(w, "gameId, environment, and X-Reason are required", http.StatusBadRequest)
			return
		}
		if err := definitionStore.Activate(r.Context(), gameID, environment, revision, claims.UserID, reason, true); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
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

func authenticateServer(r *http.Request, key ed25519.PublicKey, matchID, allocationID, build string) (matches.JoinClaim, error) {
	return matches.VerifyServerClaim(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), key, time.Now(), matchID, allocationID, build)
}

func requireScope(w http.ResponseWriter, r *http.Request, authenticate func(*http.Request) (auth.SessionClaims, error), scope string) (auth.SessionClaims, bool) {
	claims, err := authenticate(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return auth.SessionClaims{}, false
	}
	if !claims.HasScope(scope) {
		http.Error(w, "insufficient scope", http.StatusForbidden)
		return auth.SessionClaims{}, false
	}
	return claims, true
}

func readDefinitionSource(r *http.Request) ([]byte, error) {
	if r.Header.Get("Content-Type") != "application/x-yaml" && r.Header.Get("Content-Type") != "text/yaml" && r.Header.Get("Content-Type") != "application/json" {
		return nil, errors.New("content type must be application/x-yaml, text/yaml, or application/json")
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20+1))
	if err != nil {
		return nil, errors.New("invalid definition body")
	}
	if len(data) > 1<<20 {
		return nil, errors.New("definition body too large")
	}
	return data, nil
}
