package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegistryTracksRequestsWithoutDynamicLabels(t *testing.T) {
	registry := New()
	handler := registry.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/failure") {
			http.Error(w, "failure", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, path := range []string{"/v1/players/player-a", "/v1/players/player-b/failure"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	}
	recorder := httptest.NewRecorder()
	registry.Write(recorder)
	body, err := io.ReadAll(recorder.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, expected := range []string{"gameservice_http_requests_total 2", "gameservice_http_errors_total 1", "gameservice_process_uptime_seconds"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("metrics missing %q: %s", expected, text)
		}
	}
	if strings.Contains(text, "player-a") || strings.Contains(text, "player-b") {
		t.Fatal("metrics exposed dynamic player labels")
	}
}
