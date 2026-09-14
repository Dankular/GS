package nakama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCallRuntimeRPCUsesBearerAndStringPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/rpc/gameservice.profile" || r.Header.Get("Authorization") != "Bearer session" {
			t.Fatalf("unexpected request: %s %s", r.URL.Path, r.Header.Get("Authorization"))
		}
		var encoded string
		if err := json.NewDecoder(r.Body).Decode(&encoded); err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(encoded), &payload); err != nil || payload["operation"] != "get" {
			t.Fatalf("unexpected payload: %s", encoded)
		}
		_, _ = w.Write([]byte(`{"payload":"{\"userId\":\"p1\"}"}`))
	}))
	defer server.Close()
	result, err := CallRuntimeRPC(context.Background(), server.Client(), server.URL, "session", "gameservice.profile", map[string]any{"operation": "get"})
	if err != nil || result["userId"] != "p1" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}
