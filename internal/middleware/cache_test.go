package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/config"
)

func TestCacheSkipsStreamingRequest(t *testing.T) {
	// Handler to ensure we reach the next handler
	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	})

	// Use a minimal config to create middleware (nils are acceptable for this test)
	mw := Cache(nil, nil, nil, config.CacheConfig{Enabled: true})(next)

	body := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if !reached {
		t.Error("expected next handler to be reached for streaming request")
	}
	if rec.Header().Get("X-Cache") != "" {
		t.Error("streaming requests should not set X-Cache header")
	}
}
