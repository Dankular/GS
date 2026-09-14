package allocation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRESTAllocatorSendsRequiredSelectorsAndMapsResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gameserverallocation" || r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
		}
		var request allocationRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Namespace != "platform-gameservers-eu-west" || request.GameServerSelectors[0].MatchLabels["platform.game/build-id"] != BuildLabel("sha256:build") {
			t.Fatalf("selector was not preserved: %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"gameServerName":"gs-1","address":"10.0.0.1","ports":[{"name":"game","port":7000}]}`))
	}))
	defer server.Close()
	allocation, err := (RESTAllocator{Endpoint: server.URL, Namespace: "platform-gameservers-eu-west"}).Allocate(context.Background(), Selector{GameID: "arena", ModeID: "dm", Build: "sha256:build", Region: "eu-west", Protocol: "3"})
	if err != nil {
		t.Fatal(err)
	}
	if allocation.GameServer != "gs-1" || allocation.Ports["game"] != 7000 {
		t.Fatalf("unexpected allocation: %#v", allocation)
	}
}

func TestRESTAllocatorFailsClosedOnBadResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "no capacity", http.StatusConflict) }))
	defer server.Close()
	_, err := (RESTAllocator{Endpoint: server.URL, Namespace: "ns"}).Allocate(context.Background(), Selector{GameID: "g", ModeID: "m", Build: "b", Region: "r", Protocol: "p"})
	if err == nil {
		t.Fatal("expected allocator error")
	}
}
