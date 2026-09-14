package simulator

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Dankular/GameService/internal/matches"
)

var ErrUnassignedPlayer = errors.New("player is not assigned to this match")

type Lifecycle interface {
	Ready() error
	Health() error
	Shutdown() error
}
type ResultSink interface {
	Submit(context.Context, matches.ResultSubmission) error
}

type Config struct {
	MatchID      string
	AllocationID string
	Build        string
	Roster       map[string]int
	PublicKey    ed25519.PublicKey
	Lifecycle    Lifecycle
	ResultSink   ResultSink
}

type ResultRequest struct {
	Sequence      int64           `json:"sequence"`
	Payload       json.RawMessage `json:"payload"`
	PayloadDigest string          `json:"payloadDigest"`
}

type Server struct {
	config             Config
	mu                 sync.RWMutex
	ready              bool
	resultAcknowledged bool
}

func New(config Config) (*Server, error) {
	if strings.TrimSpace(config.MatchID) == "" || strings.TrimSpace(config.AllocationID) == "" || strings.TrimSpace(config.Build) == "" || len(config.Roster) == 0 {
		return nil, errors.New("simulator bootstrap configuration is incomplete")
	}
	if len(config.PublicKey) != ed25519.PublicKeySize {
		return nil, errors.New("simulator claim public key is required")
	}
	return &Server{config: config}, nil
}

func (s *Server) Bootstrap() error {
	if s.config.Lifecycle != nil {
		if err := s.config.Lifecycle.Ready(); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.ready = true
	s.mu.Unlock()
	return nil
}
func (s *Server) IsReady() bool { s.mu.RLock(); defer s.mu.RUnlock(); return s.ready }

func (s *Server) AuthorizeJoin(token string, now time.Time) (string, int, error) {
	claim, err := matches.VerifyClaim(token, s.config.PublicKey, now, "game-server", s.config.MatchID, s.config.Build)
	if err != nil {
		return "", 0, err
	}
	if claim.AllocationID != s.config.AllocationID {
		return "", 0, errors.New("allocation mismatch")
	}
	slot, ok := s.config.Roster[claim.Subject]
	if !ok || slot != claim.Slot {
		return "", 0, ErrUnassignedPlayer
	}
	return claim.Subject, slot, nil
}

func (s *Server) SubmitResult(ctx context.Context, request ResultRequest) error {
	if s.config.ResultSink == nil {
		return errors.New("result sink is not configured")
	}
	canonical, digest, err := (matches.ResultSubmission{MatchID: s.config.MatchID, Sequence: request.Sequence, Payload: request.Payload, PayloadDigest: request.PayloadDigest}).Validate()
	if err != nil {
		return err
	}
	if err := s.config.ResultSink.Submit(ctx, matches.ResultSubmission{MatchID: s.config.MatchID, Sequence: request.Sequence, Payload: canonical, PayloadDigest: digest}); err != nil {
		return err
	}
	s.mu.Lock()
	s.resultAcknowledged = true
	s.mu.Unlock()
	if s.config.Lifecycle != nil {
		return s.config.Lifecycle.Shutdown()
	}
	return nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, _ *http.Request) {
		if s.IsReady() {
			w.WriteHeader(http.StatusOK)
		} else {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
		}
	})
	mux.HandleFunc("POST /join", func(w http.ResponseWriter, r *http.Request) {
		player, slot, err := s.AuthorizeJoin(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), time.Now())
		if err != nil {
			http.Error(w, "join rejected", http.StatusForbidden)
			return
		}
		write(w, map[string]any{"accepted": true, "playerId": player, "slot": slot})
	})
	mux.HandleFunc("POST /result", func(w http.ResponseWriter, r *http.Request) {
		var request ResultRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid result", http.StatusBadRequest)
			return
		}
		if err := s.SubmitResult(r.Context(), request); err != nil {
			http.Error(w, fmt.Sprintf("result rejected: %v", err), http.StatusUnprocessableEntity)
			return
		}
		write(w, map[string]any{"accepted": true})
	})
	return mux
}

func write(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
