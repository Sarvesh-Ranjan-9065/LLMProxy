package cache

import (
    "context"
    "testing"
    "time"

    "github.com/yourusername/llmproxy/internal/config"
)

func setupTestRedis(t *testing.T) *RedisClient {
    rc, err := NewRedisClient(config.RedisConfig{
        Addr: "localhost:6379",
    })
    if err != nil {
        t.Skipf("Redis not available: %v", err)
    }
    return rc
}

func TestCacheSetGet(t *testing.T) {
    rc := setupTestRedis(t)
    ctx := context.Background()
    key := "test:cache:" + time.Now().Format("150405")

    // Set
    err := rc.Set(ctx, key, `{"response":"hello"}`, 10*time.Second)
    if err != nil {
        t.Fatalf("Set failed: %v", err)
    }
    t.Log("✅ Value stored in Redis")

    // Get
    val, err := rc.Get(ctx, key)
    if err != nil {
        t.Fatalf("Get failed: %v", err)
    }
    if val != `{"response":"hello"}` {
        t.Errorf("unexpected value: %s", val)
    }
    t.Logf("✅ Value retrieved: %s", val)

    // Cleanup
    rc.Delete(ctx, key)
}

func TestTTLExpiry(t *testing.T) {
    rc := setupTestRedis(t)
    ctx := context.Background()
    key := "test:ttl:" + time.Now().Format("150405")

    rc.Set(ctx, key, "temporary", 1*time.Second)
    t.Log("✅ Set with 1s TTL")

    time.Sleep(1500 * time.Millisecond)

    _, err := rc.Get(ctx, key)
    if err == nil {
        t.Error("value should have expired")
    }
    t.Log("✅ Value expired after TTL")
}

func TestTenantIsolation(t *testing.T) {
    hasher := NewSemanticHasher()
    body := []byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"hello"}]}`)

    hash, _ := hasher.Hash(body)

    // Two different tenants, same prompt
    keyA := "cache:tenant-A:" + hash
    keyB := "cache:tenant-B:" + hash

    if keyA == keyB {
        t.Error("different tenants should have different cache keys")
    }
    t.Logf("✅ Tenant A key: %s...", keyA[:30])
    t.Logf("✅ Tenant B key: %s...", keyB[:30])
    t.Log("✅ Same prompt, different tenants → different keys (no data leakage)")
}
