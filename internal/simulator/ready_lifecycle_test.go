package simulator

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPReadyLifecycleReportsMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/server/matches/m-1/ready" || r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("unexpected ready request: %s %s", r.Method, r.URL.String())
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	lifecycle := HTTPReadyLifecycle{ControlURL: server.URL, Token: "token", MatchID: func() string { return "m-1" }, Client: server.Client()}
	if err := lifecycle.Ready(); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPReadyLifecycleSendsHeartbeat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/server/matches/m-1/heartbeat" || r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("unexpected heartbeat request: %s %s", r.Method, r.URL.String())
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	lifecycle := HTTPReadyLifecycle{ControlURL: server.URL, Token: "token", MatchID: func() string { return "m-1" }, Client: server.Client()}
	if err := lifecycle.Heartbeat(); err != nil {
		t.Fatal(err)
	}
}
