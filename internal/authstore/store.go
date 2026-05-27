package authstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/cache"
	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/config"
	"github.com/redis/go-redis/v9"
)

const defaultRedisPrefix = "auth:apikey:"

type Store interface {
	Lookup(ctx context.Context, apiKey string) (config.APIKeyInfo, bool, error)
}

type ConfigStore struct {
	keys config.APIKeyMap
}

func NewConfigStore(keys config.APIKeyMap) *ConfigStore {
	return &ConfigStore{keys: keys}
}

func (s *ConfigStore) Lookup(_ context.Context, apiKey string) (config.APIKeyInfo, bool, error) {
	info, ok := s.keys[apiKey]
	if !ok {
		return config.APIKeyInfo{}, false, nil
	}
	return normalizeInfo(info), true, nil
}

type RedisStore struct {
	client *cache.RedisClient
	prefix string
}

func NewRedisStore(client *cache.RedisClient, prefix string) *RedisStore {
	if prefix == "" {
		prefix = defaultRedisPrefix
	}
	return &RedisStore{client: client, prefix: prefix}
}

func (s *RedisStore) Lookup(ctx context.Context, apiKey string) (config.APIKeyInfo, bool, error) {
	value, err := s.client.Get(ctx, s.key(apiKey))
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return config.APIKeyInfo{}, false, nil
		}
		return config.APIKeyInfo{}, false, err
	}
	if value == "" {
		return config.APIKeyInfo{}, false, nil
	}

	var info config.APIKeyInfo
	if err := json.Unmarshal([]byte(value), &info); err == nil {
		return normalizeInfo(info), true, nil
	}

	// Fall back to treating the raw value as an owner string.
	return normalizeInfo(config.APIKeyInfo{Owner: value, Role: "user"}), true, nil
}

func (s *RedisStore) Set(ctx context.Context, apiKey string, info config.APIKeyInfo, ttl time.Duration) error {
	data, err := json.Marshal(normalizeInfo(info))
	if err != nil {
		return fmt.Errorf("marshal auth info: %w", err)
	}
	return s.client.Set(ctx, s.key(apiKey), string(data), ttl)
}

func (s *RedisStore) key(apiKey string) string {
	apiKey = strings.TrimSpace(apiKey)
	return s.prefix + apiKey
}

func normalizeInfo(info config.APIKeyInfo) config.APIKeyInfo {
	if strings.TrimSpace(info.Owner) == "" {
		info.Owner = "unknown"
	}
	if strings.TrimSpace(info.Role) == "" {
		info.Role = "user"
	}
	return info
}

func New(cfg config.AuthConfig, redisClient *cache.RedisClient) (Store, error) {
	store := strings.ToLower(strings.TrimSpace(cfg.Store))
	if store == "" {
		store = "config"
	}

	switch store {
	case "config":
		return NewConfigStore(cfg.APIKeys), nil
	case "redis":
		if redisClient == nil {
			return nil, fmt.Errorf("redis auth store requested but redis client is nil")
		}
		return NewRedisStore(redisClient, cfg.RedisPrefix), nil
	default:
		return nil, fmt.Errorf("unknown auth store: %s", store)
	}
}
