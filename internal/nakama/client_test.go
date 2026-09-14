package nakama

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteLeaderboardRecordUsesServerAPI(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v2/leaderboard/deathmatch" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte("secret:"))
		if r.Header.Get("Authorization") != want {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"owner_id":"player-1"`) || !strings.Contains(string(body), `"score":42`) {
			t.Fatalf("body = %s", body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer s.Close()
	if err := (Client{BaseURL: s.URL, ServerKey: "secret"}).WriteLeaderboardRecord(context.Background(), "deathmatch", LeaderboardRecord{UserID: "player-1", Score: 42}); err != nil {
		t.Fatal(err)
	}
}
func TestWriteLeaderboardRecordRejectsFailure(t *testing.T) {
	s := httptest.NewServer(http.NotFoundHandler())
	defer s.Close()
	if err := (Client{BaseURL: s.URL, ServerKey: "secret"}).WriteLeaderboardRecord(context.Background(), "missing", LeaderboardRecord{UserID: "p"}); err == nil {
		t.Fatal("expected HTTP failure")
	}
}

func TestWriteTournamentRecordUsesServerRuntimeHTTPKey(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v2/rpc/gameservice.tournament_record" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("http_key") != "runtime-secret" {
			t.Fatalf("unexpected runtime query: %s", r.URL.RawQuery)
		}
		if _, ok := r.URL.Query()["unwrap"]; !ok {
			t.Fatalf("runtime query did not request unwrap: %s", r.URL.RawQuery)
		}
		body, _ := io.ReadAll(r.Body)
		for _, expected := range []string{`"tournamentId":"weekly-arena"`, `"eventKey":"match-1:1:sha256:test"`, `"ownerId":"player-1"`, `"score":42`, `"durationSeconds":3600`, `"metadata":{"matchId":"m1"}`} {
			if !strings.Contains(string(body), expected) {
				t.Fatalf("body missing %q: %s", expected, body)
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer s.Close()
	if err := (Client{BaseURL: s.URL, RuntimeHTTPKey: "runtime-secret"}).WriteTournamentRecord(context.Background(), TournamentConfig{ID: "weekly-arena", EventKey: "match-1:1:sha256:test", DurationSeconds: 3600}, LeaderboardRecord{UserID: "player-1", Score: 42, Metadata: `{"matchId":"m1"}`}); err != nil {
		t.Fatal(err)
	}
}

func TestWriteTournamentRecordRequiresRuntimeKey(t *testing.T) {
	if err := (Client{BaseURL: "http://nakama"}).WriteTournamentRecord(context.Background(), TournamentConfig{ID: "t", EventKey: "event", DurationSeconds: 1}, LeaderboardRecord{UserID: "p"}); err == nil {
		t.Fatal("expected missing runtime key error")
	}
}
