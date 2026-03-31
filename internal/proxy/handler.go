package proxy

import (
	"encoding/json"
	"net/http"
)

// HealthHandler returns the proxy's health status
func HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "healthy",
			"service": "llmproxy-gateway",
		})
	}
}

// InfoHandler returns proxy configuration info
func InfoHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"service": "LLMProxy",
			"version": "1.0.0",
			"endpoints": []string{
				"POST /v1/chat/completions",
				"GET  /health",
				"GET  /metrics",
				"GET  /info",
			},
		})
	}
}