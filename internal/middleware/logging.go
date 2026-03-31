package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
	size   int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.size += n
	return n, err
}

// Logging middleware provides structured request logging
func Logging() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			apiKey := GetAPIKey(r.Context())

			recorder := &statusRecorder{
				ResponseWriter: w,
				status:         200,
			}

			next.ServeHTTP(recorder, r)

			duration := time.Since(start)

			slog.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", recorder.status,
				"duration_ms", duration.Milliseconds(),
				"size", recorder.size,
				"api_key", maskKey(apiKey),
				"remote_addr", r.RemoteAddr,
				"user_agent", r.UserAgent(),
				"cache", w.Header().Get("X-Cache"),
			)
		})
	}
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "***" + key[len(key)-4:]
}