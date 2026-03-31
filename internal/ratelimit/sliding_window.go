package ratelimit

import (
	"context"
	"fmt"
	"time"
)

// SlidingWindow implements sliding window rate limiting using Redis sorted sets
type SlidingWindow struct {
	store *Store
}

func NewSlidingWindow(store *Store) *SlidingWindow {
	return &SlidingWindow{store: store}
}

// Allow checks if a request is allowed using sliding window algorithm
func (sw *SlidingWindow) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, int, time.Duration, error) {
	now := time.Now().UnixMicro()
	windowMicro := window.Microseconds()

	luaScript := `
		local key = KEYS[1]
		local now = tonumber(ARGV[1])
		local window = tonumber(ARGV[2])
		local limit = tonumber(ARGV[3])

		-- Remove expired entries
		local window_start = now - window
		redis.call("ZREMRANGEBYSCORE", key, "-inf", window_start)

		-- Count current entries
		local count = redis.call("ZCARD", key)

		if count < limit then
			-- Add the new request
			redis.call("ZADD", key, now, tostring(now) .. ":" .. tostring(math.random(1000000)))
			redis.call("PEXPIRE", key, math.ceil(window / 1000))
			return {1, limit - count - 1, 0}
		else
			-- Get oldest entry to calculate retry-after
			local oldest = redis.call("ZRANGE", key, 0, 0, "WITHSCORES")
			local retry_after = 0
			if #oldest > 0 then
				retry_after = tonumber(oldest[2]) + window - now
			end
			return {0, 0, retry_after}
		end
	`

	windowKey := fmt.Sprintf("ratelimit:window:%s", key)
	result, err := sw.store.Eval(ctx, luaScript, []string{windowKey}, now, windowMicro, limit)
	if err != nil {
		return true, limit, 0, fmt.Errorf("sliding window check failed: %w", err)
	}

	values, ok := result.([]interface{})
	if !ok || len(values) != 3 {
		return true, limit, 0, fmt.Errorf("unexpected sliding window response")
	}

	allowed := values[0].(int64) == 1
	remaining := int(values[1].(int64))
	retryAfterMicro := values[2].(int64)
	retryAfter := time.Duration(retryAfterMicro) * time.Microsecond

	return allowed, remaining, retryAfter, nil
}