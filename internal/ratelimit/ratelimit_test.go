package ratelimit

import (
    "context"
    "testing"
    "time"

    "github.com/Sarvesh-Ranjan-9065/llmproxy/internal/cache"
    "github.com/Sarvesh-Ranjan-9065/llmproxy/internal/config"
)

func setupRedis(t *testing.T) *Store {
    rc, err := cache.NewRedisClient(config.RedisConfig{
        Addr: "localhost:6379",
    })
    if err != nil {
        t.Skipf("Redis not available, skipping: %v", err)
    }
    return NewStore(rc)
}

func TestTokenBucketAllow(t *testing.T) {
    store := setupRedis(t)
    tb := NewTokenBucket(store)
    ctx := context.Background()

    // Use a unique key so tests don't interfere
    key := "test-allow-" + time.Now().Format("150405.000")

    // Rate: 5 req/sec, burst: 5
    for i := 0; i < 5; i++ {
        allowed, remaining, _, err := tb.Allow(ctx, key, 5, 5)
        if err != nil {
            t.Fatalf("error: %v", err)
        }
        if !allowed {
            t.Errorf("request %d should be allowed", i+1)
        }
        t.Logf("  Request %d: allowed=%v remaining=%d", i+1, allowed, remaining)
    }

    // 6th request should be denied
    allowed, _, retryAfter, _ := tb.Allow(ctx, key, 5, 5)
    if allowed {
        t.Error("6th request should be rate limited")
    }
    t.Logf("✅ 6th request denied, retry after: %v", retryAfter)
}

func TestTokenBucketRefill(t *testing.T) {
    store := setupRedis(t)
    tb := NewTokenBucket(store)
    ctx := context.Background()

    key := "test-refill-" + time.Now().Format("150405.000")

    // Exhaust all tokens (burst=2, rate=10/sec)
    tb.Allow(ctx, key, 10, 2)
    tb.Allow(ctx, key, 10, 2)

    // Should be denied now
    allowed, _, _, _ := tb.Allow(ctx, key, 10, 2)
    if allowed {
        t.Error("should be denied after exhausting burst")
    }
    t.Log("✅ Tokens exhausted — denied")

    // Wait for refill (100ms should give us 1 token at 10/sec)
    time.Sleep(150 * time.Millisecond)

    allowed, _, _, _ = tb.Allow(ctx, key, 10, 2)
    if !allowed {
        t.Error("should be allowed after token refill")
    }
    t.Log("✅ After 150ms wait — token refilled, allowed again")
}
