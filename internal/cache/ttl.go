package cache

import (
	"context"
	"time"
)

// TTLManager manages cache TTL policies
type TTLManager struct {
	defaultTTL time.Duration
	redis      *RedisClient
}

func NewTTLManager(redis *RedisClient, defaultTTL time.Duration) *TTLManager {
	return &TTLManager{
		defaultTTL: defaultTTL,
		redis:      redis,
	}
}

func (t *TTLManager) DefaultTTL() time.Duration {
	return t.defaultTTL
}

// GetWithTTL retrieves a cached value and its remaining TTL
func (t *TTLManager) GetWithTTL(ctx context.Context, key string) (string, time.Duration, error) {
	val, err := t.redis.Get(ctx, key)
	if err != nil {
		return "", 0, err
	}

	ttl, err := t.redis.TTL(ctx, key)
	if err != nil {
		return val, 0, nil
	}

	return val, ttl, nil
}

// SetWithTTL stores a value with TTL
func (t *TTLManager) SetWithTTL(ctx context.Context, key string, value string, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = t.defaultTTL
	}
	return t.redis.Set(ctx, key, value, ttl)
}

// Invalidate removes a cached entry
func (t *TTLManager) Invalidate(ctx context.Context, key string) error {
	return t.redis.Delete(ctx, key)
}