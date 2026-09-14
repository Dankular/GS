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
