package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAllowRefillsAndBoundsBuckets(t *testing.T) {
	l := New(2)
	now := time.Unix(100, 0)
	if !l.Allow("one", 1, 2, now) || !l.Allow("one", 1, 2, now) {
		t.Fatal("burst was not allowed")
	}
	if l.Allow("one", 1, 2, now) {
		t.Fatal("burst limit was not enforced")
	}
	if !l.Allow("one", 1, 2, now.Add(time.Second)) {
		t.Fatal("bucket did not refill")
	}
	if !l.Allow("two", 1, 1, now) {
		t.Fatal("second bucket was not allowed")
	}
	if l.Allow("three", 1, 1, now) {
		t.Fatal("bounded limiter admitted beyond capacity")
	}
}

func TestMiddlewareSeparatesExpensivePathAndSetsRetryAfter(t *testing.T) {
	c := Config{IPRate: 100, IPBurst: 100, ActorRate: 100, ActorBurst: 100, ExpensiveRate: 0, ExpensiveBurst: 0}
	// A disabled rate (zero) is useful for health checks and explicit tests.
	called := 0
	handler := c.Middleware(New(10), http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called++ }))
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "http://example/v1/commands", nil)
	request.RemoteAddr = "192.0.2.1:1234"
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || called != 1 {
		t.Fatalf("disabled expensive limiter rejected request: code=%d called=%d", response.Code, called)
	}

	c.ExpensiveRate, c.ExpensiveBurst = 1, 1
	l := c.Middleware(New(10), http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called++ }))
	first := httptest.NewRecorder()
	l.ServeHTTP(first, request)
	second := httptest.NewRecorder()
	l.ServeHTTP(second, request)
	if second.Code != http.StatusTooManyRequests || second.Header().Get("Retry-After") == "" {
		t.Fatalf("expensive request was not limited: code=%d retry=%q", second.Code, second.Header().Get("Retry-After"))
	}
}

func TestMiddlewareAppliesIPLimitAcrossOperations(t *testing.T) {
	c := Config{IPRate: 100, IPBurst: 1, ActorRate: 100, ActorBurst: 100, ExpensiveRate: 100, ExpensiveBurst: 100}
	handler := c.Middleware(New(10), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	first := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://example/v1/players/me/snapshot", nil)
	request.RemoteAddr = "192.0.2.9:1234"
	handler.ServeHTTP(first, request)
	second := httptest.NewRecorder()
	request.URL.Path = "/metrics"
	handler.ServeHTTP(second, request)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("IP limit was bypassed by changing operation: code=%d", second.Code)
	}
}
