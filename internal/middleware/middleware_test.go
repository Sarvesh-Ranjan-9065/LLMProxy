package middleware

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/Sarvesh-Ranjan-9065/llmproxy/internal/config"
)

func TestAuthMissingKey(t *testing.T) {
    cfg := config.AuthConfig{
        Enabled: true,
        APIKeys: map[string]string{"test-key": "test-user"},
    }

    handler := Auth(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        t.Error("should not reach handler")
    }))

    req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
    rec := httptest.NewRecorder()
    handler.ServeHTTP(rec, req)

    if rec.Code != http.StatusUnauthorized {
        t.Errorf("expected 401, got %d", rec.Code)
    }
    t.Logf("✅ No API key → %d", rec.Code)
}

func TestAuthInvalidKey(t *testing.T) {
    cfg := config.AuthConfig{
        Enabled: true,
        APIKeys: map[string]string{"test-key": "test-user"},
    }

    handler := Auth(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        t.Error("should not reach handler")
    }))

    req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
    req.Header.Set("X-API-Key", "wrong-key")
    rec := httptest.NewRecorder()
    handler.ServeHTTP(rec, req)

    if rec.Code != http.StatusUnauthorized {
        t.Errorf("expected 401, got %d", rec.Code)
    }
    t.Logf("✅ Invalid API key → %d", rec.Code)
}

func TestAuthValidKey(t *testing.T) {
    cfg := config.AuthConfig{
        Enabled: true,
        APIKeys: map[string]string{"test-key": "test-user"},
    }

    var gotKey string
    handler := Auth(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        gotKey = GetAPIKey(r.Context())
        w.WriteHeader(200)
    }))

    req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
    req.Header.Set("X-API-Key", "test-key")
    rec := httptest.NewRecorder()
    handler.ServeHTTP(rec, req)

    if rec.Code != 200 {
        t.Errorf("expected 200, got %d", rec.Code)
    }
    if gotKey != "test-key" {
        t.Errorf("expected key in context, got %s", gotKey)
    }
    t.Logf("✅ Valid key → %d, context has key: %s", rec.Code, gotKey)
}

func TestAuthBearerToken(t *testing.T) {
    cfg := config.AuthConfig{
        Enabled: true,
        APIKeys: map[string]string{"my-token": "bearer-user"},
    }

    var gotKey string
    handler := Auth(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        gotKey = GetAPIKey(r.Context())
        w.WriteHeader(200)
    }))

    req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
    req.Header.Set("Authorization", "Bearer my-token")
    rec := httptest.NewRecorder()
    handler.ServeHTTP(rec, req)

    if rec.Code != 200 {
        t.Errorf("expected 200, got %d", rec.Code)
    }
    t.Logf("✅ Bearer token auth works: key=%s", gotKey)
}

func TestAuthDisabled(t *testing.T) {
    cfg := config.AuthConfig{Enabled: false}

    reached := false
    handler := Auth(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        reached = true
        w.WriteHeader(200)
    }))

    req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
    // No API key at all
    rec := httptest.NewRecorder()
    handler.ServeHTTP(rec, req)

    if !reached {
        t.Error("handler should be reached when auth is disabled")
    }
    t.Log("✅ Auth disabled → request passes through")
}

func TestRecoveryFromPanic(t *testing.T) {
    handler := Recovery()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        panic("something broke!")
    }))

    req := httptest.NewRequest("GET", "/test", nil)
    rec := httptest.NewRecorder()
    handler.ServeHTTP(rec, req)

    if rec.Code != 500 {
        t.Errorf("expected 500, got %d", rec.Code)
    }
    contentType := rec.Header().Get("Content-Type")
    if contentType != "application/json" {
        t.Errorf("expected application/json, got %s", contentType)
    }
    t.Logf("✅ Panic recovered → %d with JSON body", rec.Code)
    t.Logf("✅ Response: %s", rec.Body.String())
}
