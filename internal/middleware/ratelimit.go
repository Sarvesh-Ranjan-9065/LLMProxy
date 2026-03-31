package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/yourusername/llmproxy/internal/config"
	"github.com/yourusername/llmproxy/internal/metrics"
	"github.com/yourusername/llmproxy/internal/ratelimit"
)

// RateLimit middleware applies per-API-key token bucket rate limiting
func RateLimit(bucket *ratelimit.TokenBucket, cfg config.RateLimitConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := GetAPIKey(r.Context())

			// Determine rate and burst for this key
			rate := cfg.DefaultRate
			burst := cfg.DefaultBurst

			if keyLimit, exists := cfg.PerKeyLimits[apiKey]; exists {
				rate = keyLimit.Rate
				burst = keyLimit.Burst
			}

			allowed, remaining, retryAfter, err := bucket.Allow(r.Context(), apiKey, rate, burst)
			if err != nil {
				// Fail open — allow the request but log the error
				slog.Error("rate limit check failed — failing open",
					"error", err,
					"api_key", apiKey,
					"rate", rate,
					"burst", burst,
				)
			}

			// Set rate limit headers
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(burst))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

			if !allowed {
				metrics.RateLimitedTotal.WithLabelValues(apiKey).Inc()

				w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
				w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(
					time.Now().Add(retryAfter).Unix(), 10,
				))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]interface{}{
						"message":     "rate limit exceeded",
						"type":        "rate_limit_error",
						"code":        429,
						"retry_after": retryAfter.Seconds(),
					},
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}