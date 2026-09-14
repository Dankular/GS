package metrics

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

type Registry struct {
	started  time.Time
	requests atomic.Uint64
	errors   atomic.Uint64
}

func New() *Registry { return &Registry{started: time.Now()} }

func (r *Registry) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		capture := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		r.requests.Add(1)
		next.ServeHTTP(capture, request)
		if capture.status >= http.StatusInternalServerError {
			r.errors.Add(1)
		}
	})
}

func (r *Registry) Write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	uptime := time.Since(r.started).Seconds()
	_, _ = fmt.Fprintf(w, "# HELP gameservice_control_api_up Control API process availability.\n# TYPE gameservice_control_api_up gauge\ngameservice_control_api_up 1\n")
	_, _ = fmt.Fprintf(w, "# HELP gameservice_http_requests_total HTTP requests received.\n# TYPE gameservice_http_requests_total counter\ngameservice_http_requests_total %d\n", r.requests.Load())
	_, _ = fmt.Fprintf(w, "# HELP gameservice_http_errors_total HTTP responses with status 5xx.\n# TYPE gameservice_http_errors_total counter\ngameservice_http_errors_total %d\n", r.errors.Load())
	_, _ = fmt.Fprintf(w, "# HELP gameservice_process_uptime_seconds Process uptime.\n# TYPE gameservice_process_uptime_seconds gauge\ngameservice_process_uptime_seconds %.3f\n", uptime)
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(body []byte) (int, error) {
	return w.ResponseWriter.Write(body)
}
