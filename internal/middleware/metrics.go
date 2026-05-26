package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/metrics"
)

type usagePayload struct {
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type metricsRecorder struct {
	http.ResponseWriter
	status    int
	size      int
	body      *bytes.Buffer
	bodyLimit int
}

func newMetricsRecorder(w http.ResponseWriter, bodyLimit int) *metricsRecorder {
	var buf *bytes.Buffer
	if bodyLimit > 0 {
		buf = &bytes.Buffer{}
	}
	return &metricsRecorder{
		ResponseWriter: w,
		status:         http.StatusOK,
		body:           buf,
		bodyLimit:      bodyLimit,
	}
}

func (r *metricsRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *metricsRecorder) Write(b []byte) (int, error) {
	if r.body != nil && r.bodyLimit > 0 {
		remaining := r.bodyLimit - r.body.Len()
		if remaining > 0 {
			if len(b) > remaining {
				r.body.Write(b[:remaining])
			} else {
				r.body.Write(b)
			}
		}
	}
	n, err := r.ResponseWriter.Write(b)
	r.size += n
	return n, err
}

// Metrics middleware records Prometheus metrics for each request
func Metrics() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			apiKey := GetAPIKey(r.Context())

			metrics.ActiveConnections.Inc()
			defer metrics.ActiveConnections.Dec()

			recorder := newMetricsRecorder(w, 1024*1024)

			next.ServeHTTP(recorder, r)

			duration := time.Since(start).Seconds()
			status := strconv.Itoa(recorder.status)

			metrics.RequestsTotal.WithLabelValues(
				r.Method, r.URL.Path, status,
			).Inc()

			// If per-key metrics are enabled, also emit the per-key counter.
			// This avoids high-cardinality labels by default.
			if metrics.PerKeyRequestsTotal != nil {
				metrics.PerKeyRequestsTotal.WithLabelValues(r.Method, r.URL.Path, status, apiKey).Inc()
			}

			metrics.RequestDuration.WithLabelValues(
				r.Method, r.URL.Path, status,
			).Observe(duration)

			promptTokens, completionTokens, totalTokens, ok := parseUsage(recorder)
			if ok {
				metrics.TokensUsed.WithLabelValues("prompt").Add(float64(promptTokens))
				metrics.TokensUsed.WithLabelValues("completion").Add(float64(completionTokens))
				metrics.TokensUsed.WithLabelValues("total").Add(float64(totalTokens))

				cost := float64(totalTokens) / 1000.0 * 0.002
				metrics.EstimatedCost.WithLabelValues().Add(cost)
				return
			}

			// Estimate token cost (rough estimation based on response size)
			estimatedTokens := float64(recorder.size) / 4.0 // ~4 chars per token
			metrics.TokensUsed.WithLabelValues("completion_estimated").Add(estimatedTokens)

			// Estimate cost: ~$0.002 per 1K tokens for GPT-3.5
			cost := estimatedTokens / 1000.0 * 0.002
			metrics.EstimatedCost.WithLabelValues().Add(cost)
		})
	}
}

func parseUsage(recorder *metricsRecorder) (int, int, int, bool) {
	if recorder == nil || recorder.body == nil {
		return 0, 0, 0, false
	}

	contentType := recorder.Header().Get("Content-Type")
	if contentType != "" && !strings.Contains(strings.ToLower(contentType), "application/json") {
		return 0, 0, 0, false
	}

	if recorder.body.Len() == 0 {
		return 0, 0, 0, false
	}

	var payload usagePayload
	if err := json.Unmarshal(recorder.body.Bytes(), &payload); err != nil {
		return 0, 0, 0, false
	}

	promptTokens := payload.Usage.PromptTokens
	completionTokens := payload.Usage.CompletionTokens
	totalTokens := payload.Usage.TotalTokens
	if totalTokens == 0 && (promptTokens > 0 || completionTokens > 0) {
		totalTokens = promptTokens + completionTokens
	}
	if totalTokens == 0 && promptTokens == 0 && completionTokens == 0 {
		return 0, 0, 0, false
	}

	return promptTokens, completionTokens, totalTokens, true
}
