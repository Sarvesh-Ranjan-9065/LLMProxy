package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/yourusername/llmproxy/internal/metrics"
)

// Metrics middleware records Prometheus metrics for each request
func Metrics() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			apiKey := GetAPIKey(r.Context())

			metrics.ActiveConnections.Inc()
			defer metrics.ActiveConnections.Dec()

			recorder := &statusRecorder{
				ResponseWriter: w,
				status:         200,
			}

			next.ServeHTTP(recorder, r)

			duration := time.Since(start).Seconds()
			status := strconv.Itoa(recorder.status)

			metrics.RequestsTotal.WithLabelValues(
				r.Method, r.URL.Path, status, apiKey,
			).Inc()

			metrics.RequestDuration.WithLabelValues(
				r.Method, r.URL.Path, status,
			).Observe(duration)

			// Estimate token cost (rough estimation based on response size)
			estimatedTokens := float64(recorder.size) / 4.0 // ~4 chars per token
			metrics.TokensUsed.WithLabelValues(apiKey, "completion").Add(estimatedTokens)

			// Estimate cost: ~$0.002 per 1K tokens for GPT-3.5
			cost := estimatedTokens / 1000.0 * 0.002
			metrics.EstimatedCost.WithLabelValues(apiKey).Add(cost)
		})
	}
}