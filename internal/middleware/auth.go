package middleware

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/config"
	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/metrics"
)

type contextKey string

const APIKeyContextKey contextKey = "api_key"
const OwnerContextKey contextKey = "api_key_owner"

// Auth validates API keys from the X-API-Key header
func Auth(cfg config.AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			// Check for API key in header
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				// Also check Authorization: Bearer <key>
				auth := r.Header.Get("Authorization")
				if len(auth) > 7 && auth[:7] == "Bearer " {
					apiKey = auth[7:]
				}
			}

			if apiKey == "" {
				metrics.AuthRejectedTotal.WithLabelValues("missing_key").Inc()
				writeError(w, http.StatusUnauthorized, "missing API key - set X-API-Key header")
				return
			}

			owner, exists := cfg.APIKeys[apiKey]
			if !exists {
				metrics.AuthRejectedTotal.WithLabelValues("invalid_key").Inc()
				writeError(w, http.StatusUnauthorized, "invalid API key")
				return
			}

			// Store API key and owner in context
			ctx := context.WithValue(r.Context(), APIKeyContextKey, apiKey)
			ctx = context.WithValue(ctx, OwnerContextKey, owner)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetAPIKey(ctx context.Context) string {
	if key, ok := ctx.Value(APIKeyContextKey).(string); ok {
		return key
	}
	return "unknown"
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message": msg,
			"type":    "authentication_error",
			"code":    status,
		},
	})
}