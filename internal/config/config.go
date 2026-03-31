package config

import (
	"encoding/json"
	"log/slog"
	"os"
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
			PerKeyLimits: map[string]KeyLimit{
				"key-premium": {Rate: 100, Burst: 200},
				"key-free":    {Rate: 2, Burst: 5},
				"key-basic":   {Rate: 10, Burst: 20},
			},
		},
		Cache: CacheConfig{
			Enabled: true,
			TTL:     5 * time.Minute,
		},
		Auth: AuthConfig{
			Enabled: true,
			APIKeys: map[string]string{
				"key-premium": "premium-user",
				"key-free":    "free-user",
				"key-basic":   "basic-user",
				"test-key":    "test-user",
			},
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
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}