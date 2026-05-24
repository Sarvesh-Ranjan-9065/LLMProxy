package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/authctx"
	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/config"
	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/metrics"
)

type requestBodyKey string

const RequestBodyContextKey requestBodyKey = "request_body"

// Auth validates API keys from the X-API-Key header
func Auth(cfg config.AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.Enabled {
				ctx := authctx.WithAuth(r.Context(), "anonymous", "anonymous", "admin", "dev")
				next.ServeHTTP(w, r.WithContext(ctx))
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

			info, exists := cfg.APIKeys[apiKey]
			if !exists {
				metrics.AuthRejectedTotal.WithLabelValues("invalid_key").Inc()
				writeError(w, http.StatusUnauthorized, "invalid API key")
				return
			}

			owner := info.Owner
			if owner == "" {
				owner = "unknown"
			}
			role := strings.ToLower(strings.TrimSpace(info.Role))
			if role == "" {
				role = "user"
			}

			// Store API key and owner in context
			ctx := authctx.WithAuth(r.Context(), apiKey, owner, role, info.Tier)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetAPIKey(ctx context.Context) string {
	return authctx.GetAPIKey(ctx)
}

func GetOwner(ctx context.Context) string {
	return authctx.GetOwner(ctx)
}

func GetRole(ctx context.Context) string {
	return authctx.GetRole(ctx)
}

func GetTier(ctx context.Context) string {
	return authctx.GetTier(ctx)
}

func getRequestBody(ctx context.Context) []byte {
	if body, ok := ctx.Value(RequestBodyContextKey).([]byte); ok {
		return body
	}
	return nil
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
