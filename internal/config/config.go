package config

import (
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"time"
)

type Config struct {
	Server    ServerConfig    `json:"server"`
	Redis     RedisConfig     `json:"redis"`
	Workers   []WorkerConfig  `json:"workers"`
	RateLimit RateLimitConfig `json:"rate_limit"`
	Cache     CacheConfig     `json:"cache"`
	Auth      AuthConfig      `json:"auth"`
}

type ServerConfig struct {
	Port            string        `json:"port"`
	ReadTimeout     time.Duration `json:"read_timeout"`
	WriteTimeout    time.Duration `json:"write_timeout"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout"`
	// Backend auth injection/passthrough
	BackendAuthHeader      string `json:"backend_auth_header,omitempty"`
	BackendAuthValue       string `json:"backend_auth_value,omitempty"`
	BackendAuthPassthrough bool   `json:"backend_auth_passthrough,omitempty"`
}

type RedisConfig struct {
	Addr     string `json:"addr"`
	Password string `json:"password"`
	DB       int    `json:"db"`
}

type WorkerConfig struct {
	URL    string `json:"url"`
	Weight int    `json:"weight"`
}

type RateLimitConfig struct {
	DefaultRate  float64             `json:"default_rate"`
	DefaultBurst int                 `json:"default_burst"`
	PerKeyLimits map[string]KeyLimit `json:"per_key_limits"`
}

type KeyLimit struct {
	Rate  float64 `json:"rate"`
	Burst int     `json:"burst"`
}

type CacheConfig struct {
	Enabled bool          `json:"enabled"`
	TTL     time.Duration `json:"ttl"`
}

type AuthConfig struct {
	Enabled bool              `json:"enabled"`
	APIKeys map[string]string `json:"api_keys"`
}

func Load() *Config {
	cfg := &Config{
		Server: ServerConfig{
			Port:            getEnv("PROXY_PORT", "8080"),
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    60 * time.Second,
			ShutdownTimeout: 10 * time.Second,
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       0,
		},
		Workers: []WorkerConfig{
			{URL: getEnv("WORKER_1_URL", "http://localhost:9001"), Weight: 1},
			{URL: getEnv("WORKER_2_URL", "http://localhost:9002"), Weight: 1},
			{URL: getEnv("WORKER_3_URL", "http://localhost:9003"), Weight: 1},
		},
		RateLimit: RateLimitConfig{
			DefaultRate:  10,
			DefaultBurst: 20,
			PerKeyLimits: map[string]KeyLimit{},
		},
		Cache: CacheConfig{
			Enabled: true,
			TTL:     5 * time.Minute,
		},
		Auth: AuthConfig{
			Enabled: true,
			APIKeys: map[string]string{},
		},
	}

	// Try loading from config file
	if configFile := getEnv("CONFIG_FILE", ""); configFile != "" {
		data, err := os.ReadFile(configFile)
		if err != nil {
			slog.Error("failed to read config file",
				"file", configFile,
				"error", err,
			)
		} else if err := json.Unmarshal(data, cfg); err != nil {
			slog.Error("failed to parse config file — using defaults",
				"file", configFile,
				"error", err,
			)
		} else {
			slog.Info("loaded config from file", "file", configFile)
		}

		// Allow API keys to be provided via environment as JSON map
		if apiKeysJSON := getEnv("API_KEYS_JSON", ""); apiKeysJSON != "" {
			var m map[string]string
			if err := json.Unmarshal([]byte(apiKeysJSON), &m); err == nil {
				cfg.Auth.APIKeys = m
			} else {
				slog.Error("invalid API_KEYS_JSON, ignoring", "error", err)
			}
		}

		// Backend auth settings from env
		cfg.Server.BackendAuthHeader = getEnv("PROXY_BACKEND_AUTH_HEADER", cfg.Server.BackendAuthHeader)
		cfg.Server.BackendAuthValue = getEnv("PROXY_BACKEND_AUTH_VALUE", cfg.Server.BackendAuthValue)
		if v := getEnv("PROXY_BACKEND_AUTH_PASSTHROUGH", ""); v != "" {
			cfg.Server.BackendAuthPassthrough = (v == "1" || strings.ToLower(v) == "true")
		}
	}
	return cfg
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
