package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/config"
	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/metrics"
	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/ratelimit"
)

// RateLimit middleware applies per-API-key token bucket rate limiting
func RateLimit(bucket *ratelimit.TokenBucket, cfg config.RateLimitConfig) func(http.Handler) http.Handler {
	fallback := ratelimit.NewInMemoryLimiter()

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
				metrics.RateLimitRedisUp.Set(0)
				slog.Warn("rate limit redis error, using local limiter",
					"error", err,
					"api_key", apiKey,
					"rate", rate,
					"burst", burst,
				)
				allowed, remaining, retryAfter = fallback.Allow(apiKey, rate, burst)
			} else {
				metrics.RateLimitRedisUp.Set(1)
			}

			// Set rate limit headers
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(burst))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

			if !allowed {
				metrics.RateLimitedTotal.WithLabelValues().Inc()

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
