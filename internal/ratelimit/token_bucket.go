package ratelimit

import (
	"context"
	"fmt"
	"time"
)

// TokenBucket implements the token bucket rate limiting algorithm using Redis
type TokenBucket struct {
	store *Store
}

func NewTokenBucket(store *Store) *TokenBucket {
	return &TokenBucket{store: store}
}

// Allow checks if a request is allowed under the rate limit.
// Returns (allowed, remaining tokens, time until next token, error)
func (tb *TokenBucket) Allow(ctx context.Context, key string, rate float64, burst int) (bool, int, time.Duration, error) {
	now := time.Now().UnixMicro()

	// Lua script for atomic token bucket operation
	// This is the heart of our distributed rate limiter
	luaScript := `
		local key = KEYS[1]
		local rate = tonumber(ARGV[1])         -- tokens per second
		local burst = tonumber(ARGV[2])        -- max tokens
		local now = tonumber(ARGV[3])          -- current time in microseconds
		local requested = 1                    -- tokens requested

		local data = redis.call("HMGET", key, "tokens", "last_time")
		local tokens = tonumber(data[1])
		local last_time = tonumber(data[2])

		if tokens == nil then
			-- First request: initialize bucket
			tokens = burst
			last_time = now
		end

		-- Calculate time passed and add tokens
		local delta = math.max(0, now - last_time)
		local delta_seconds = delta / 1000000.0
		tokens = math.min(burst, tokens + (delta_seconds * rate))

		local allowed = 0
		local remaining = math.floor(tokens)

		if tokens >= requested then
			tokens = tokens - requested
			allowed = 1
			remaining = math.floor(tokens)
		end

		-- Calculate retry after
		local retry_after = 0
		if allowed == 0 then
			retry_after = math.ceil((requested - tokens) / rate * 1000000)
		end

		-- Save state
		redis.call("HMSET", key, "tokens", tostring(tokens), "last_time", tostring(now))
		redis.call("EXPIRE", key, math.ceil(burst / rate) + 1)

		return {allowed, remaining, retry_after}
	`

	bucketKey := fmt.Sprintf("ratelimit:bucket:%s", key)
	result, err := tb.store.Eval(ctx, luaScript, []string{bucketKey}, rate, burst, now)
	if err != nil {
		// If Redis is down, allow the request (fail open)
		return true, burst, 0, fmt.Errorf("rate limit check failed: %w", err)
	}

	values, ok := result.([]interface{})
	if !ok || len(values) != 3 {
		return true, burst, 0, fmt.Errorf("unexpected rate limit response")
	}

	allowed := values[0].(int64) == 1
	remaining := int(values[1].(int64))
	retryAfterMicro := values[2].(int64)
	retryAfter := time.Duration(retryAfterMicro) * time.Microsecond

	return allowed, remaining, retryAfter, nil
}