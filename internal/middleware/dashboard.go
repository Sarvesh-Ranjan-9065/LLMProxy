package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/dashboard"
)

type dashboardRecorder struct {
	http.ResponseWriter
	status int
}

func (r *dashboardRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// DashboardStats records per-key stats for the dashboard views.
func DashboardStats(store *dashboard.Store) func(http.Handler) http.Handler {
	if store == nil {
		return func(next http.Handler) http.Handler { return next }
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			model := ""
			if shouldParseModel(r) {
				body := getRequestBody(r.Context())
				if body == nil {
					readBody, err := readRequestBody(r)
					if err == nil {
						body = readBody
						ctx := context.WithValue(r.Context(), RequestBodyContextKey, body)
						r = r.WithContext(ctx)
					}
				}
				model = extractModel(body)
			}

			recorder := &dashboardRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r)

			cacheStatus := recorder.Header().Get("X-Cache")
			store.Record(dashboard.RequestInfo{
				APIKey:    GetAPIKey(r.Context()),
				Owner:     GetOwner(r.Context()),
				Role:      GetRole(r.Context()),
				Tier:      GetTier(r.Context()),
				Method:    r.Method,
				Path:      r.URL.Path,
				Status:    recorder.status,
				LatencyMs: float64(time.Since(start).Milliseconds()),
				Cache:     cacheStatus,
				Model:     model,
				When:      time.Now(),
			})
		})
	}
}

func shouldParseModel(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}

	path := r.URL.Path
	if !strings.HasSuffix(path, "/chat/completions") && !strings.HasSuffix(path, "/completions") {
		return false
	}

	contentType := r.Header.Get("Content-Type")
	return strings.HasPrefix(contentType, "application/json") || contentType == ""
}

func readRequestBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func extractModel(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	var payload struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Model)
}
