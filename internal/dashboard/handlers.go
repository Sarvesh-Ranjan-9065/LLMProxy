package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/authctx"
	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/cache"
	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/router"
)

type Handler struct {
	store *Store
	pool  *router.Pool
	redis *cache.RedisClient
}

func NewHandler(store *Store, pool *router.Pool, redis *cache.RedisClient) *Handler {
	return &Handler{
		store: store,
		pool:  pool,
		redis: redis,
	}
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	apiKey := authctx.GetAPIKey(r.Context())
	owner := authctx.GetOwner(r.Context())
	role := authctx.GetRole(r.Context())
	tier := authctx.GetTier(r.Context())

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"owner":    owner,
		"role":     role,
		"tier":     tier,
		"key_hint": maskKey(apiKey),
	})
}

func (h *Handler) UserSummary(w http.ResponseWriter, r *http.Request) {
	apiKey := authctx.GetAPIKey(r.Context())
	summary := h.store.GetUserSummary(apiKey)
	writeJSON(w, http.StatusOK, summary)
}

func (h *Handler) AdminSummary(w http.ResponseWriter, r *http.Request) {
	summary := h.store.GetAdminSummary(5)
	summary.Backends = h.backendStatuses()
	summary.Redis = h.redisStatus(r.Context())
	writeJSON(w, http.StatusOK, summary)
}

func (h *Handler) backendStatuses() []BackendStatus {
	if h.pool == nil {
		return []BackendStatus{}
	}

	backends := h.pool.GetBackends()
	out := make([]BackendStatus, 0, len(backends))
	for _, backend := range backends {
		out = append(out, BackendStatus{
			URL:               backend.URL.String(),
			Alive:             backend.IsAlive(),
			ActiveConnections: backend.GetConnections(),
		})
	}
	return out
}

func (h *Handler) redisStatus(ctx context.Context) RedisStatus {
	if h.redis == nil {
		return RedisStatus{Healthy: false, Error: "redis not configured"}
	}

	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	if err := h.redis.Ping(ctx); err != nil {
		return RedisStatus{Healthy: false, Error: err.Error()}
	}
	return RedisStatus{Healthy: true}
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "***" + key[len(key)-4:]
}
