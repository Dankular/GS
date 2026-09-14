package gameservice

import (
	"context"
	"net/http"
	"testing"
)

func TestGeneratedGetMatchRequestUsesDeclaredPathParameter(t *testing.T) {
	req, err := NewGetMatchRequest("https://gameservice.example", "match/one")
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != http.MethodGet {
		t.Fatalf("method = %q", req.Method)
	}
	if got, want := req.URL.String(), "https://gameservice.example/v1/matches/match%2Fone"; got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
}

func TestGeneratedRequestEditorCanAttachBearerToken(t *testing.T) {
	var seen *http.Request
	client, err := NewClient("https://gameservice.example", WithHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		seen = req
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	})), WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer test")
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetMatch(context.Background(), "match"); err != nil {
		t.Fatal(err)
	}
	if seen == nil || seen.Header.Get("Authorization") != "Bearer test" {
		t.Fatalf("request editor did not attach bearer token: %#v", seen)
	}
}

func TestGeneratedDryRunDefinitionRequestEncodesImpactQuery(t *testing.T) {
	gameID := "arena"
	revision := 4
	req, err := NewDryRunDefinitionRequest("https://gameservice.example", &DryRunDefinitionParams{GameId: &gameID, Revision: &revision})
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != http.MethodPost {
		t.Fatalf("method = %q", req.Method)
	}
	if got, want := req.URL.String(), "https://gameservice.example/v1/admin/definitions/dry-run?gameId=arena&revision=4"; got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }
