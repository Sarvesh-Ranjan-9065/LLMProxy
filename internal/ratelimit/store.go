package ratelimit

import (
	"context"

	"github.com/yourusername/llmproxy/internal/cache"
)

// Store wraps Redis operations for rate limiting
type Store struct {
	redis *cache.RedisClient
}

func NewStore(redis *cache.RedisClient) *Store {
	return &Store{redis: redis}
}

func (s *Store) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	return s.redis.Eval(ctx, script, keys, args...)
}