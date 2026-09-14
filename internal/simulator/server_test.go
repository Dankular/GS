package simulator

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Dankular/GameService/internal/matches"
)

type fakeLifecycle struct{ ready, shutdown bool }

func (f *fakeLifecycle) Ready() error    { f.ready = true; return nil }
func (f *fakeLifecycle) Health() error   { return nil }
func (f *fakeLifecycle) Shutdown() error { f.shutdown = true; return nil }

type fakeSink struct {
	submission matches.ResultSubmission
	token      string
}

type fakeStarter struct{ starts int }

func (f *fakeStarter) Start() error {
	f.starts++
	return nil
}

func (f *fakeSink) Submit(_ context.Context, submission matches.ResultSubmission) error {
	f.submission = submission
	return nil
}
func (f *fakeSink) SetToken(token string) { f.token = token }

func newSimulator(t *testing.T) (*Server, ed25519.PrivateKey, *fakeLifecycle, *fakeSink) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := &fakeLifecycle{}
	sink := &fakeSink{}
	server, err := New(Config{MatchID: "m", AllocationID: "a", Build: "b", Roster: map[string]int{"player": 2}, PublicKey: public, Lifecycle: lifecycle, ResultSink: sink})
	if err != nil {
		t.Fatal(err)
	}
	return server, private, lifecycle, sink
}

func TestSimulatorBootstrapsAndRejectsUnassignedPlayer(t *testing.T) {
	server, private, lifecycle, _ := newSimulator(t)
	if err := server.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	if !server.IsReady() || !lifecycle.ready {
		t.Fatal("server did not become ready")
	}
	claim := matches.JoinClaim{Issuer: "control-plane", Audience: "game-server", Subject: "other", MatchID: "m", AllocationID: "a", ServerBuild: "b", IssuedAt: 10, NotBefore: 10, ExpiresAt: 20, JTI: "j"}
	token, err := matches.SignClaim(claim, private)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.AuthorizeJoin(token, time.Unix(10, 0)); err != ErrUnassignedPlayer {
		t.Fatalf("expected roster rejection, got %v", err)
	}
}

func TestSimulatorSubmitsCanonicalResultAndShutsDown(t *testing.T) {
	server, _, lifecycle, sink := newSimulator(t)
	if err := server.SubmitResult(context.Background(), ResultRequest{Sequence: 1, Payload: []byte(` { "score": 4 } `)}); err != nil {
		t.Fatal(err)
	}
	if !lifecycle.shutdown || string(sink.submission.Payload) != `{"score":4}` {
		t.Fatalf("result was not finalized: %#v", sink.submission)
	}
}

func TestDynamicAssignmentUpdatesJoinAuthorization(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := &fakeLifecycle{}
	server, err := New(Config{PublicKey: public, Lifecycle: lifecycle, DynamicAssignment: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	if err := server.Assign("dynamic-match", "dynamic-allocation", "build-2", map[string]int{"player": 1}); err != nil {
		t.Fatal(err)
	}
	claim := matches.JoinClaim{Issuer: "control-plane", Audience: "game-server", Subject: "player", MatchID: "dynamic-match", AllocationID: "dynamic-allocation", ServerBuild: "build-2", Slot: 1, IssuedAt: 10, NotBefore: 10, ExpiresAt: 20, JTI: "dynamic-jti"}
	token, err := matches.SignClaim(claim, private)
	if err != nil {
		t.Fatal(err)
	}
	if player, slot, err := server.AuthorizeJoin(token, time.Unix(10, 0)); err != nil || player != "player" || slot != 1 {
		t.Fatalf("dynamic assignment did not authorize player: %q %d %v", player, slot, err)
	}
}

func TestDynamicAssignmentUpdatesServerToken(t *testing.T) {
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sink := &fakeSink{}
	server, err := New(Config{PublicKey: public, Lifecycle: &fakeLifecycle{}, ResultSink: sink, DynamicAssignment: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.AssignWithServerToken("m", "a", "b", map[string]int{"p": 0}, "token"); err != nil {
		t.Fatal(err)
	}
	if sink.token != "token" {
		t.Fatalf("expected dynamic server token, got %q", sink.token)
	}
}

func TestJoinStartsMatchAfterClaimAuthorization(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	starter := &fakeStarter{}
	server, err := New(Config{MatchID: "m", AllocationID: "a", Build: "b", Roster: map[string]int{"player": 0}, PublicKey: public, Lifecycle: &fakeLifecycle{}, ResultSink: &fakeSink{}, Starter: starter})
	if err != nil {
		t.Fatal(err)
	}
	claim := matches.JoinClaim{Issuer: "control-plane", Audience: "game-server", Subject: "player", MatchID: "m", AllocationID: "a", ServerBuild: "b", Slot: 0, IssuedAt: time.Now().Unix(), NotBefore: time.Now().Add(-time.Second).Unix(), ExpiresAt: time.Now().Add(time.Minute).Unix(), JTI: "join-jti"}
	token, err := matches.SignClaim(claim, private)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/join", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || starter.starts != 1 {
		t.Fatalf("join did not start match: status=%d starts=%d", response.Code, starter.starts)
	}
}
