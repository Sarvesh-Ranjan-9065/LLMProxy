package config

import (
    "os"
    "testing"
)

func TestLoadDefaults(t *testing.T) {
    cfg := Load()

    if cfg.Server.Port != "8080" {
        t.Errorf("expected port 8080, got %s", cfg.Server.Port)
    }
    if cfg.Redis.Addr != "localhost:6379" {
        t.Errorf("expected redis localhost:6379, got %s", cfg.Redis.Addr)
    }
    if len(cfg.Auth.APIKeys) != 4 {
        t.Errorf("expected 4 API keys, got %d", len(cfg.Auth.APIKeys))
    }
    if cfg.Cache.Enabled != true {
        t.Error("expected cache enabled")
    }

    t.Logf("✅ Port: %s", cfg.Server.Port)
    t.Logf("✅ Redis: %s", cfg.Redis.Addr)
    t.Logf("✅ Workers: %d", len(cfg.Workers))
    t.Logf("✅ API Keys: %v", cfg.Auth.APIKeys)
    t.Logf("✅ Rate limit default: %.0f req/sec", cfg.RateLimit.DefaultRate)
}

func TestLoadFromEnv(t *testing.T) {
    os.Setenv("PROXY_PORT", "9999")
    os.Setenv("REDIS_ADDR", "redis.example.com:6379")
    defer os.Unsetenv("PROXY_PORT")
    defer os.Unsetenv("REDIS_ADDR")

    cfg := Load()

    if cfg.Server.Port != "9999" {
        t.Errorf("expected port 9999, got %s", cfg.Server.Port)
    }
    if cfg.Redis.Addr != "redis.example.com:6379" {
        t.Errorf("expected custom redis addr, got %s", cfg.Redis.Addr)
    }
    t.Logf("✅ Env override works: port=%s redis=%s", cfg.Server.Port, cfg.Redis.Addr)
}

func TestBadConfigFile(t *testing.T) {
    os.Setenv("CONFIG_FILE", "/nonexistent/path.json")
    defer os.Unsetenv("CONFIG_FILE")

    cfg := Load()

    // Should not crash, should use defaults
    if cfg.Server.Port == "" {
        t.Error("config should fall back to defaults")
    }
    t.Log("✅ Bad config file handled gracefully, defaults used")
}
