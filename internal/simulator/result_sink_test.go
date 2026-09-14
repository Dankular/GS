package simulator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Dankular/GameService/internal/matches"
)

func TestHTTPResultSinkPostsMatchScopedResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/server/matches/m/results" || r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("unexpected result request: %s %s", r.URL, r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	err := (HTTPResultSink{ControlURL: server.URL, Token: "token"}).Submit(context.Background(), matches.ResultSubmission{MatchID: "m", Sequence: 1, Payload: []byte(`{"score":1}`), PayloadDigest: matches.Digest([]byte(`{"score":1}`))})
	if err != nil {
		t.Fatal(err)
	}
}
