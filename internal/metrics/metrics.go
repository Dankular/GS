package metrics

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

type Registry struct {
	started         time.Time
	requests        atomic.Uint64
	errors          atomic.Uint64
	durationCount   atomic.Uint64
	durationNanos   atomic.Uint64
	durationBuckets [6]atomic.Uint64
}

var durationBucketSeconds = [...]float64{0.005, 0.025, 0.1, 0.25, 1, 5}

func New() *Registry { return &Registry{started: time.Now()} }

func (r *Registry) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		capture := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		started := time.Now()
		r.requests.Add(1)
		next.ServeHTTP(capture, request)
		r.observe(time.Since(started))
		if capture.status >= http.StatusInternalServerError {
			r.errors.Add(1)
		}
	})
}

func (r *Registry) observe(duration time.Duration) {
	r.durationCount.Add(1)
	r.durationNanos.Add(uint64(duration))
	seconds := duration.Seconds()
	for index, bound := range durationBucketSeconds {
		if seconds <= bound {
			r.durationBuckets[index].Add(1)
		}
	}
}

func (r *Registry) Write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	uptime := time.Since(r.started).Seconds()
	_, _ = fmt.Fprintf(w, "# HELP gameservice_control_api_up Control API process availability.\n# TYPE gameservice_control_api_up gauge\ngameservice_control_api_up 1\n")
	_, _ = fmt.Fprintf(w, "# HELP gameservice_http_requests_total HTTP requests received.\n# TYPE gameservice_http_requests_total counter\ngameservice_http_requests_total %d\n", r.requests.Load())
	_, _ = fmt.Fprintf(w, "# HELP gameservice_http_errors_total HTTP responses with status 5xx.\n# TYPE gameservice_http_errors_total counter\ngameservice_http_errors_total %d\n", r.errors.Load())
	_, _ = fmt.Fprintln(w, "# HELP gameservice_http_request_duration_seconds HTTP request duration in seconds.")
	_, _ = fmt.Fprintln(w, "# TYPE gameservice_http_request_duration_seconds histogram")
	for index, bound := range durationBucketSeconds {
		_, _ = fmt.Fprintf(w, "gameservice_http_request_duration_seconds_bucket{le=\"%.3f\"} %d\n", bound, r.durationBuckets[index].Load())
	}
	_, _ = fmt.Fprintf(w, "gameservice_http_request_duration_seconds_bucket{le=\"+Inf\"} %d\n", r.durationCount.Load())
	_, _ = fmt.Fprintf(w, "gameservice_http_request_duration_seconds_sum %.9f\n", float64(r.durationNanos.Load())/float64(time.Second))
	_, _ = fmt.Fprintf(w, "gameservice_http_request_duration_seconds_count %d\n", r.durationCount.Load())
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
