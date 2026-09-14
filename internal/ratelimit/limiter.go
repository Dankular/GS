package ratelimit

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type bucket struct {
	tokens float64
	seen   time.Time
}

// Limiter is a bounded, process-local limiter. It is intentionally placed at
// the edge of the API; a distributed deployment should configure an upstream
// gateway limiter as well.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]bucket
	maxKeys int
}

func New(maxKeys int) *Limiter {
	if maxKeys < 1 {
		maxKeys = 10000
	}
	return &Limiter{buckets: make(map[string]bucket), maxKeys: maxKeys}
}

func (l *Limiter) Allow(key string, ratePerSecond float64, burst int, now time.Time) bool {
	if ratePerSecond <= 0 || burst <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	state, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= l.maxKeys {
			l.evict(now)
			if len(l.buckets) >= l.maxKeys {
				return false
			}
		}
		state = bucket{tokens: float64(burst), seen: now}
	}
	if elapsed := now.Sub(state.seen).Seconds(); elapsed > 0 {
		state.tokens += elapsed * ratePerSecond
		if state.tokens > float64(burst) {
			state.tokens = float64(burst)
		}
		state.seen = now
	}
	if state.tokens < 1 {
		l.buckets[key] = state
		return false
	}
	state.tokens--
	l.buckets[key] = state
	return true
}

func (l *Limiter) evict(now time.Time) {
	cutoff := now.Add(-10 * time.Minute)
	for key, state := range l.buckets {
		if state.seen.Before(cutoff) {
			delete(l.buckets, key)
		}
	}
}

type Config struct {
	IPRate         float64
	IPBurst        int
	ActorRate      float64
	ActorBurst     int
	ExpensiveRate  float64
	ExpensiveBurst int
}

func DefaultConfig() Config {
	return Config{IPRate: 2, IPBurst: 60, ActorRate: 4, ActorBurst: 120, ExpensiveRate: 1, ExpensiveBurst: 30}
}

func (c Config) Middleware(l *Limiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		path := r.URL.Path
		ip := remoteIP(r.RemoteAddr)
		allowed := l.Allow("ip:"+ip, c.IPRate, c.IPBurst, now)
		allowed = allowed && l.Allow("ip:"+ip+"|path:"+path, c.IPRate, c.IPBurst, now)
		if token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")); token != "" {
			digest := sha256.Sum256([]byte(token))
			actor := hex.EncodeToString(digest[:])
			allowed = allowed && l.Allow("actor:"+actor, c.ActorRate, c.ActorBurst, now)
			allowed = allowed && l.Allow("actor:"+actor+"|path:"+path, c.ActorRate, c.ActorBurst, now)
		}
		if strings.HasPrefix(path, "/v1/commands") || strings.HasPrefix(path, "/v1/admin/definitions") || strings.HasPrefix(path, "/v1/matchmaking/") {
			allowed = allowed && l.Allow("expensive:"+ip+"|path:"+path, c.ExpensiveRate, c.ExpensiveBurst, now)
		}
		if !allowed {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func remoteIP(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	if remoteAddr == "" {
		return "unknown"
	}
	return remoteAddr
}
