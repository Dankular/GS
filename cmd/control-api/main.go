package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	admincommands "github.com/Dankular/GameService/internal/admin"
	"github.com/Dankular/GameService/internal/auth"
	"github.com/Dankular/GameService/internal/commands"
	"github.com/Dankular/GameService/internal/commandstore"
	"github.com/Dankular/GameService/internal/compiler"
	"github.com/Dankular/GameService/internal/definitions"
	"github.com/Dankular/GameService/internal/economy"
	"github.com/Dankular/GameService/internal/matches"
	"github.com/Dankular/GameService/internal/matchmaking"
	telemetry "github.com/Dankular/GameService/internal/metrics"
	"github.com/Dankular/GameService/internal/nakama"
	tracing "github.com/Dankular/GameService/internal/telemetry"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	shutdownTelemetry, err := tracing.Setup(context.Background(), "gameservice.control-api", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if err != nil {
		slog.Error("telemetry initialization failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = shutdownTelemetry(context.Background()) }()
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
	resultFinalizer := matches.ResultFinalizer{Economy: economyService}
	matchmakingStore := matchmaking.Store{Pool: repository.Pool()}
	matchmakingCommandService := matchmaking.CommandService{Store: matchmakingStore}
	definitionStore := definitions.Store{Pool: repository.Pool()}
	definitionCommandService := definitions.CommandService{Store: definitionStore}
	adminCommandService := admincommands.CommandService{}
	joinPrivateKeyBytes, _ := base64.RawStdEncoding.DecodeString(os.Getenv("JOIN_CLAIM_PRIVATE_KEY"))
	var joinPrivateKey ed25519.PrivateKey
	if len(joinPrivateKeyBytes) == ed25519.PrivateKeySize {
		joinPrivateKey = ed25519.PrivateKey(joinPrivateKeyBytes)
	}
	matchStore := matches.Store{Pool: repository.Pool(), JoinPrivateKey: joinPrivateKey, Issuer: "control-plane", Audience: "game-server"}
	matchCommandService := matches.CommandService{Store: matchStore}
	sessionSigningKey := os.Getenv("NAKAMA_SESSION_SIGNING_KEY")
	if sessionSigningKey == "" {
		slog.Error("NAKAMA_SESSION_SIGNING_KEY is required")
		os.Exit(1)
	}
	issuer := os.Getenv("NAKAMA_SESSION_ISSUER")
	audience := os.Getenv("NAKAMA_SESSION_AUDIENCE")
	serverPublicKeys := decodePublicKeys(os.Getenv("SERVER_CLAIM_PUBLIC_KEYS"))
	if len(serverPublicKeys) == 0 {
		if key, err := base64.RawStdEncoding.DecodeString(os.Getenv("SERVER_CLAIM_PUBLIC_KEY")); err == nil && len(key) == ed25519.PublicKeySize {
			serverPublicKeys = []ed25519.PublicKey{ed25519.PublicKey(key)}
		}
	}
	commandHandler := func(ctx context.Context, tx pgx.Tx, envelope commands.Envelope) (commands.Result, error) {
		if strings.HasPrefix(envelope.Spec.Operation, "match.") {
			return matchCommandService.Handle(ctx, tx, envelope)
		}
		if strings.HasPrefix(envelope.Spec.Operation, "matchmaking.") {
			return matchmakingCommandService.Handle(ctx, tx, envelope)
		}
		if strings.HasPrefix(envelope.Spec.Operation, "definition.") {
			return definitionCommandService.Handle(ctx, tx, envelope)
		}
		if strings.HasPrefix(envelope.Spec.Operation, "admin.") {
			return adminCommandService.Handle(ctx, tx, envelope)
		}
		return economyService.Handle(ctx, tx, envelope)
	}
	adminCommandService.Handler = admincommands.Handler(commandHandler)
	nakamaURL := os.Getenv("NAKAMA_URL")
	if nakamaURL == "" {
		nakamaURL = "http://nakama:7350"
	}
	nakamaRuntime := nakama.Client{BaseURL: nakamaURL, RuntimeHTTPKey: os.Getenv("NAKAMA_RUNTIME_HTTP_KEY")}
	metricRegistry := telemetry.New()
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
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		metricRegistry.Write(w)
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
	mux.HandleFunc("POST /v1/players/me/privacy/export", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := requireScope(w, r, authenticate, "player:read")
		if !ok {
			return
		}
		requestID, ok := privacyRequestID(w, r)
		if !ok {
			return
		}
		var replay json.RawMessage
		if err := repository.Pool().QueryRow(r.Context(), `SELECT result FROM platform.account_privacy_requests WHERE request_id=$1 AND player_id=$2 AND operation='export' AND status='completed'`, requestID, claims.UserID).Scan(&replay); err == nil {
			w.Header().Set("X-Idempotency-Replay", "true")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(replay)
			return
		}
		var account map[string]any
		if err := nakamaRuntime.RuntimeRPC(r.Context(), "gameservice.privacy", map[string]any{"operation": "export", "userId": claims.UserID}, &account); err != nil {
			http.Error(w, "account export unavailable", http.StatusUnprocessableEntity)
			return
		}
		snapshot, err := economy.Snapshot(r.Context(), repository.Pool(), claims.UserID)
		if err != nil {
			http.Error(w, "account export unavailable", http.StatusServiceUnavailable)
			return
		}
		result := map[string]any{"requestId": requestID, "playerId": claims.UserID, "nakama": account["account"], "gameservice": snapshot}
		if _, err := repository.Pool().Exec(r.Context(), `INSERT INTO platform.account_privacy_requests(request_id,player_id,operation,status,result) VALUES($1,$2,'export','completed',$3) ON CONFLICT (request_id) DO NOTHING`, requestID, claims.UserID, result); err != nil {
			http.Error(w, "account export could not be recorded", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, result)
	})
	mux.HandleFunc("POST /v1/players/me/privacy/delete", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := requireScope(w, r, authenticate, "player:write")
		if !ok {
			return
		}
		requestID, ok := privacyRequestID(w, r)
		if !ok {
			return
		}
		hashID := privacyHash(claims.UserID)
		var replay json.RawMessage
		if err := repository.Pool().QueryRow(r.Context(), `SELECT result FROM platform.account_privacy_requests WHERE request_id=$1 AND player_id=$2 AND operation='delete' AND status='completed'`, requestID, hashID).Scan(&replay); err == nil {
			w.Header().Set("X-Idempotency-Replay", "true")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(replay)
			return
		}
		var existing bool
		if err := repository.Pool().QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM platform.deleted_account_tombstones WHERE player_id_hash=$1)`, hashID).Scan(&existing); err != nil {
			http.Error(w, "account deletion unavailable", 503)
			return
		}
		if existing {
			writeJSON(w, map[string]any{"requestId": requestID, "deleted": true, "replayed": true})
			return
		}
		var deleted map[string]any
		if err := nakamaRuntime.RuntimeRPC(r.Context(), "gameservice.privacy", map[string]any{"operation": "delete", "userId": claims.UserID}, &deleted); err != nil {
			http.Error(w, "account deletion unavailable", http.StatusUnprocessableEntity)
			return
		}
		tx, err := repository.Pool().Begin(r.Context())
		if err != nil {
			http.Error(w, "account deletion unavailable", 503)
			return
		}
		defer tx.Rollback(r.Context())
		if _, err = tx.Exec(r.Context(), `INSERT INTO platform.account_privacy_requests(request_id,player_id,operation,status,result) VALUES($1,$2,'delete','completed','{"deleted":true}'::jsonb) ON CONFLICT (request_id) DO NOTHING`, requestID, hashID); err != nil {
			http.Error(w, "account deletion unavailable", 503)
			return
		}
		for _, query := range []string{`UPDATE economy.ledger_entries SET player_id=$1 WHERE player_id=$2`, `DELETE FROM economy.wallet_accounts WHERE player_id=$1`, `DELETE FROM economy.inventory_stacks WHERE player_id=$1`, `DELETE FROM economy.entitlements WHERE player_id=$1`, `DELETE FROM progression.player_progress WHERE player_id=$1`, `DELETE FROM progression.objective_completions WHERE player_id=$1`, `DELETE FROM economy.reward_claims WHERE player_id=$1`, `DELETE FROM platform.player_restrictions WHERE player_id=$1`, `DELETE FROM match.join_claims WHERE player_id=$1`, `DELETE FROM match.ticket_members WHERE player_id=$1`, `UPDATE platform.command_requests SET actor_id=$1 WHERE actor_id=$2`} {
			if strings.HasPrefix(query, "UPDATE") {
				_, err = tx.Exec(r.Context(), query, "deleted:"+hashID, claims.UserID)
			} else {
				_, err = tx.Exec(r.Context(), query, claims.UserID)
			}
			if err != nil {
				http.Error(w, "account deletion unavailable", 503)
				return
			}
		}
		if _, err = tx.Exec(r.Context(), `UPDATE match.results SET payload=jsonb_set(payload,'{players}',COALESCE((SELECT jsonb_agg(elem - 'playerId') FROM jsonb_array_elements(CASE WHEN jsonb_typeof(payload->'players')='array' THEN payload->'players' ELSE '[]'::jsonb END) elem),'[]'::jsonb),true) WHERE payload ? 'players' AND EXISTS (SELECT 1 FROM jsonb_array_elements(payload->'players') elem WHERE elem->>'playerId'=$1)`, claims.UserID); err != nil {
			http.Error(w, "account deletion unavailable", 503)
			return
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO platform.deleted_account_tombstones(player_id_hash,request_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, hashID, requestID); err != nil {
			http.Error(w, "account deletion unavailable", 503)
			return
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO ops.audit_log(actor_type,actor_id,action,resource_type,resource_id,correlation_id,details) VALUES('system',$1,'account.delete','player',$1,$2,'{"tombstoned":true}'::jsonb)`, "deleted:"+hashID, requestID); err != nil {
			http.Error(w, "account deletion unavailable", 503)
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			http.Error(w, "account deletion unavailable", 503)
			return
		}
		writeJSON(w, map[string]any{"requestId": requestID, "deleted": true, "tombstone": "deleted:" + hashID})
	})
	mux.HandleFunc("GET /v1/matches/{matchId}", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := requireScope(w, r, authenticate, "player:read")
		if !ok {
			return
		}
		record, err := matchStore.Get(r.Context(), r.PathValue("matchId"), claims.UserID)
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "match not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "match unavailable", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, record)
	})
	mux.HandleFunc("POST /v1/matches/{matchId}/join-claims", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := requireScope(w, r, authenticate, "player:read")
		if !ok {
			return
		}
		token, err := matchStore.IssueJoinClaim(r.Context(), r.PathValue("matchId"), claims.UserID, time.Now())
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "match or roster member not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "join claims unavailable", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, map[string]any{"matchId": r.PathValue("matchId"), "token": token})
	})
	mux.HandleFunc("GET /v1/commands/{requestId}", func(w http.ResponseWriter, r *http.Request) {
		claims, err := authenticate(r)
		if err != nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		var result commands.Result
		if claims.HasScope("admin:read") {
			result, err = repository.Get(r.Context(), r.PathValue("requestId"))
		} else {
			result, err = repository.GetForActor(r.Context(), r.PathValue("requestId"), claims.UserID)
		}
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
		if scope := commandScope(e.Spec.Operation); scope != "" && !sessionAllowsScope(claims, scope) {
			http.Error(w, "insufficient scope", http.StatusForbidden)
			return
		}
		if e.Spec.Operation == "profile.get" || e.Spec.Operation == "profile.patch_public_fields" {
			stored, lookupErr := repository.GetForActor(r.Context(), e.Metadata.RequestID, claims.UserID)
			if lookupErr == nil {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Idempotency-Replay", "true")
				writeJSON(w, stored)
				return
			}
			if !errors.Is(lookupErr, pgx.ErrNoRows) {
				http.Error(w, "command could not be loaded", http.StatusServiceUnavailable)
				return
			}
			rpcOperation := "get"
			rpcPayload := map[string]any{"operation": rpcOperation}
			if e.Spec.Operation == "profile.patch_public_fields" {
				rpcOperation = "patch_public_fields"
				rpcPayload["operation"] = rpcOperation
				rpcPayload["fields"] = e.Spec.Arguments["fields"]
			}
			profileData, rpcErr := nakama.CallRuntimeRPC(r.Context(), http.DefaultClient, nakamaURL, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), "gameservice.profile", rpcPayload)
			if rpcErr != nil {
				slog.Error("profile RPC failed", "requestId", e.Metadata.RequestID, "error", rpcErr)
				http.Error(w, "profile operation unavailable", http.StatusServiceUnavailable)
				return
			}
			profileResult := commands.Result{RequestID: e.Metadata.RequestID, CorrelationID: e.Metadata.CorrelationID, Status: "succeeded", Operation: e.Spec.Operation, Result: profileData, Events: []string{"profile.updated.v1"}}
			if rpcOperation == "get" {
				profileResult.Events = []string{"profile.read.v1"}
			}
			result, duplicate, submitErr := repository.SubmitWith(r.Context(), e, func(context.Context, pgx.Tx, commands.Envelope) (commands.Result, error) { return profileResult, nil })
			if submitErr != nil {
				http.Error(w, "command could not be stored", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if duplicate {
				w.Header().Set("X-Idempotency-Replay", "true")
			}
			writeJSON(w, result)
			return
		}
		result, duplicate, err := repository.SubmitWith(r.Context(), e, commandHandler)
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
			if errors.Is(err, matchmaking.ErrActiveTicket) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
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
		if len(serverPublicKeys) == 0 {
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
		// Completed is accepted here only so ResultFinalizer can inspect the
		// existing sequence and return an idempotent duplicate. A new sequence
		// still fails inside the transactional state transition.
		if !resultStateAccepts(state) {
			http.Error(w, "match is not accepting results", http.StatusConflict)
			return
		}
		_, err := authenticateServer(r, serverPublicKeys, matchID, expectedAllocation, expectedBuild)
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
		duplicate, digest, err := resultFinalizer.Submit(r.Context(), tx, matches.ResultSubmission{MatchID: matchID, Sequence: request.Sequence, Payload: request.Payload, PayloadDigest: request.PayloadDigest, CorrelationID: r.Header.Get("X-Correlation-ID")})
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
	mux.HandleFunc("POST /v1/server/matches/{matchId}/ready", func(w http.ResponseWriter, r *http.Request) {
		if err := verifyServerRequest(r, repository, serverPublicKeys, r.PathValue("matchId")); err != nil {
			writeServerError(w, err)
			return
		}
		if err := matchStore.MarkReady(r.Context(), r.PathValue("matchId")); errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "match is not allocating", http.StatusConflict)
			return
		} else if err != nil {
			http.Error(w, "match ready update failed", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /v1/server/matches/{matchId}/start", func(w http.ResponseWriter, r *http.Request) {
		if err := verifyServerRequest(r, repository, serverPublicKeys, r.PathValue("matchId")); err != nil {
			writeServerError(w, err)
			return
		}
		if err := matchStore.StartRunning(r.Context(), r.PathValue("matchId")); errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "match is not ready", http.StatusConflict)
			return
		} else if err != nil {
			http.Error(w, "match start update failed", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /v1/server/matches/{matchId}/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		if err := verifyServerRequest(r, repository, serverPublicKeys, r.PathValue("matchId")); err != nil {
			writeServerError(w, err)
			return
		}
		if err := matchStore.Heartbeat(r.Context(), r.PathValue("matchId")); errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "match is not active", http.StatusConflict)
			return
		} else if err != nil {
			http.Error(w, "heartbeat failed", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
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
	mux.HandleFunc("POST /v1/admin/definitions/{revision}/rollback", func(w http.ResponseWriter, r *http.Request) {
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
		if err := definitionStore.Rollback(r.Context(), gameID, environment, revision, claims.UserID, reason, true); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /v1/admin/audit", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireScope(w, r, authenticate, "admin:read"); !ok {
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		records, err := definitionStore.Audit(r.Context(), limit)
		if err != nil {
			http.Error(w, "audit unavailable", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, map[string]any{"records": records})
	})
	slog.Info("control API listening", "addr", addr)
	if err := http.ListenAndServe(addr, otelhttp.NewHandler(metricRegistry.Middleware(mux), "gameservice.control-api")); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func privacyRequestID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if id == "" || len(id) > 128 {
		http.Error(w, "Idempotency-Key is required and must be at most 128 characters", http.StatusBadRequest)
		return "", false
	}
	return id, true
}

func privacyHash(playerID string) string {
	sum := sha256.Sum256([]byte("gameservice-account-tombstone:" + playerID))
	return fmt.Sprintf("%x", sum[:])
}

func resultStateAccepts(state string) bool {
	return state == "Running" || state == "Finalizing" || state == "Completed"
}

func authenticateServer(r *http.Request, keys []ed25519.PublicKey, matchID, allocationID, build string) (matches.JoinClaim, error) {
	return matches.VerifyServerClaimAny(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), keys, time.Now(), matchID, allocationID, build)
}

func requireScope(w http.ResponseWriter, r *http.Request, authenticate func(*http.Request) (auth.SessionClaims, error), scope string) (auth.SessionClaims, bool) {
	claims, err := authenticate(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return auth.SessionClaims{}, false
	}
	if !sessionAllowsScope(claims, scope) {
		http.Error(w, "insufficient scope", http.StatusForbidden)
		return auth.SessionClaims{}, false
	}
	return claims, true
}

func sessionAllowsScope(claims auth.SessionClaims, scope string) bool {
	if claims.HasScope(scope) {
		return true
	}
	// Nakama's ordinary player session token has no scope claim. It is still
	// allowed to use self-service player APIs; privileged scopes remain explicit.
	return (scope == "player:read" || scope == "player:write") && claims.Scope == "" && len(claims.Vars) == 0
}

func commandScope(operation string) string {
	switch operation {
	case "definition.validate":
		return "definition:validate"
	case "definition.publish":
		return "definition:publish"
	case "definition.activate", "definition.rollback":
		return "definition:activate"
	case "admin.audit_search":
		return "admin:read"
	case "admin.player_snapshot":
		return "admin:read"
	case "admin.execute_command":
		return "admin:write"
	case "admin.player_restrict", "admin.player_unrestrict":
		return "admin:write"
	}
	switch operation {
	case "profile.get", "inventory.list", "wallet.get", "entitlement.list", "progression.get", "reward.preview", "match.get", "matchmaking.status":
		return "player:read"
	case "profile.patch_public_fields", "inventory.grant", "inventory.consume", "inventory.transfer", "wallet.credit", "wallet.debit", "wallet.transfer", "entitlement.grant", "entitlement.revoke", "progression.add_xp", "progression.complete_objective", "reward.claim", "matchmaking.enqueue", "matchmaking.cancel", "match.issue_join_claim", "match.abandon":
		return "player:write"
	default:
		return ""
	}
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

func verifyServerRequest(r *http.Request, repository *commandstore.Repository, keys []ed25519.PublicKey, matchID string) error {
	if len(keys) == 0 {
		return errors.New("server claim verification is not configured")
	}
	var build, allocation, state string
	if err := repository.Pool().QueryRow(r.Context(), `SELECT server_build,COALESCE(allocation_id,''),state FROM match.matches WHERE match_id=$1`, matchID).Scan(&build, &allocation, &state); err != nil {
		return err
	}
	if _, err := authenticateServer(r, keys, matchID, allocation, build); err != nil {
		return err
	}
	return nil
}

func decodePublicKeys(value string) []ed25519.PublicKey {
	var keys []ed25519.PublicKey
	for _, encoded := range strings.Split(value, ",") {
		key, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(encoded))
		if err == nil && len(key) == ed25519.PublicKeySize {
			keys = append(keys, ed25519.PublicKey(key))
		}
	}
	return keys
}

func writeServerError(w http.ResponseWriter, err error) {
	if strings.Contains(err.Error(), "not configured") {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "match not found", http.StatusNotFound)
		return
	}
	http.Error(w, fmt.Sprintf("invalid server claim: %v", err), http.StatusUnauthorized)
}
