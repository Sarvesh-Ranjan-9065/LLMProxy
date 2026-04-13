package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/config"
	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/router"
)

func TestReverseProxyForwardsPathQueryAndHeaders(t *testing.T) {
	var gotPath string
	var gotQuery string
	var gotMethod string
	var gotForwardedFor string

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotMethod = r.Method
		gotForwardedFor = r.Header.Get("X-Forwarded-For")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer backend.Close()

	pool, err := router.NewPool([]config.WorkerConfig{{URL: backend.URL, Weight: 1}})
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	lb := router.NewLoadBalancer(pool, router.RoundRobin)
	rp := NewReverseProxy(lb)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?model=test", strings.NewReader(`{"msg":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "1.1.1.1")
	req.RemoteAddr = "2.2.2.2:12345"

	rec := httptest.NewRecorder()
	rp.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotPath != "/v1/chat/completions" {
		t.Errorf("expected path /v1/chat/completions, got %s", gotPath)
	}
	if gotQuery != "model=test" {
		t.Errorf("expected query model=test, got %s", gotQuery)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("expected method POST, got %s", gotMethod)
	}
	if !strings.Contains(gotForwardedFor, "1.1.1.1") || !strings.Contains(gotForwardedFor, "2.2.2.2") {
		t.Errorf("expected X-Forwarded-For to include prior and client IP, got %s", gotForwardedFor)
	}
	if rec.Header().Get("X-Backend") == "" {
		t.Error("expected X-Backend header to be set")
	}
}

func TestReverseProxyReturns503WhenNoBackends(t *testing.T) {
	pool, err := router.NewPool(nil)
	if err != nil {
		t.Fatalf("unexpected pool error: %v", err)
	}

	lb := router.NewLoadBalancer(pool, router.RoundRobin)
	rp := NewReverseProxy(lb)

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	rp.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "no backends available") {
		t.Errorf("expected no backends message, got %s", rec.Body.String())
	}
}
