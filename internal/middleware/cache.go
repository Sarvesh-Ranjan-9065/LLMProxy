package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	internalCache "github.com/yourusername/llmproxy/internal/cache"
	"github.com/yourusername/llmproxy/internal/config"
	"github.com/yourusername/llmproxy/internal/metrics"
)

// responseRecorder captures the response so we can cache it
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

func newResponseRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		body:           &bytes.Buffer{},
	}
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.statusCode = code
	rr.ResponseWriter.WriteHeader(code)
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	rr.body.Write(b) // Capture response body
	return rr.ResponseWriter.Write(b)
}

// Cache middleware checks Redis for cached responses and stores new ones.
// Cache keys are namespaced by API key so tenants never share cached data.
func Cache(
	redisClient *internalCache.RedisClient,
	hasher *internalCache.SemanticHasher,
	ttlMgr *internalCache.TTLManager,
	cfg config.CacheConfig,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			// Only cache POST requests
			if r.Method != http.MethodPost {
				next.ServeHTTP(w, r)
				return
			}

			// Only cache chat/completions endpoints
			path := r.URL.Path
			if !strings.HasSuffix(path, "/chat/completions") &&
				!strings.HasSuffix(path, "/completions") {
				next.ServeHTTP(w, r)
				return
			}

			// Read body
			body, err := io.ReadAll(r.Body)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			r.Body = io.NopCloser(bytes.NewBuffer(body))

			// Check if streaming is requested — don't cache streams
			var req internalCache.ChatRequest
			if err := json.Unmarshal(body, &req); err == nil && req.Stream {
				next.ServeHTTP(w, r)
				return
			}

			// Generate content hash
			contentHash, err := hasher.Hash(body)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			// Namespace cache key by tenant (API key) to prevent data leakage
			apiKey := GetAPIKey(r.Context())
			cacheKey := fmt.Sprintf("cache:%s:%s", apiKey, contentHash)

			// Check cache
			cached, err := redisClient.Get(r.Context(), cacheKey)
			if err == nil && cached != "" {
				// Cache HIT
				metrics.CacheHitsTotal.Inc()
				slog.Debug("cache hit", "key", cacheKey[:30])

				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Cache", "HIT")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(cached))
				return
			}

			// Cache MISS
			metrics.CacheMissesTotal.Inc()
			w.Header().Set("X-Cache", "MISS")

			// Record the response
			recorder := newResponseRecorder(w)
			next.ServeHTTP(recorder, r)

			// Only cache successful responses
			if recorder.statusCode == http.StatusOK {
				// Use a detached background context so the write succeeds
				// even after the request context is cancelled.
				responseBody := recorder.body.String()
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					if err := ttlMgr.SetWithTTL(ctx, cacheKey, responseBody, cfg.TTL); err != nil {
						slog.Error("failed to cache response",
							"error", err,
							"key", cacheKey[:30],
						)
					}
				}()
			}
		})
	}
}