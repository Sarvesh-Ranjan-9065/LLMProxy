package router

import (
    "testing"

    "github.com/yourusername/llmproxy/internal/config"
)

func TestPoolCreation(t *testing.T) {
    workers := []config.WorkerConfig{
        {URL: "http://localhost:9001", Weight: 1},
        {URL: "http://localhost:9002", Weight: 2},
        {URL: "http://localhost:9003", Weight: 1},
    }

    pool, err := NewPool(workers)
    if err != nil {
        t.Fatalf("failed to create pool: %v", err)
    }

    backends := pool.GetBackends()
    if len(backends) != 3 {
        t.Errorf("expected 3 backends, got %d", len(backends))
    }

    // Verify it returns a copy (modifying shouldn't affect pool)
    backends[0] = nil
    original := pool.GetBackends()
    if original[0] == nil {
        t.Error("GetBackends should return a copy, not internal slice")
    }

    t.Logf("✅ Pool created with %d backends", len(original))
    t.Log("✅ GetBackends returns a safe copy")
}

func TestRoundRobin(t *testing.T) {
    workers := []config.WorkerConfig{
        {URL: "http://localhost:9001", Weight: 1},
        {URL: "http://localhost:9002", Weight: 1},
    }
    pool, _ := NewPool(workers)
    lb := NewLoadBalancer(pool, RoundRobin)

    seen := map[string]int{}
    for i := 0; i < 10; i++ {
        b, err := lb.Next()
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }
        seen[b.URL.String()]++
    }

    t.Logf("✅ Distribution: %v", seen)
    if len(seen) < 2 {
        t.Error("round robin should hit both backends")
    }
}

func TestWeightedZeroWeight(t *testing.T) {
    workers := []config.WorkerConfig{
        {URL: "http://localhost:9001", Weight: 0},
        {URL: "http://localhost:9002", Weight: 0},
    }
    pool, _ := NewPool(workers)
    lb := NewLoadBalancer(pool, Weighted)

    // This should NOT panic (was a bug before the fix)
    b, err := lb.Next()
    if err != nil {
        t.Fatalf("should not error: %v", err)
    }
    t.Logf("✅ Zero-weight fallback works, got: %s", b.URL.String())
}

func TestNoBackends(t *testing.T) {
    workers := []config.WorkerConfig{
        {URL: "http://localhost:9001", Weight: 1},
    }
    pool, _ := NewPool(workers)

    // Mark all as dead
    for _, b := range pool.GetBackends() {
        b.SetAlive(false)
    }

    lb := NewLoadBalancer(pool, RoundRobin)
    _, err := lb.Next()

    if err != ErrNoBackendsAvailable {
        t.Error("should return ErrNoBackendsAvailable when all backends are down")
    }
    t.Log("✅ No backends → proper error returned")
}

func TestLeastConnections(t *testing.T) {
    workers := []config.WorkerConfig{
        {URL: "http://localhost:9001", Weight: 1},
        {URL: "http://localhost:9002", Weight: 1},
    }
    pool, _ := NewPool(workers)
    lb := NewLoadBalancer(pool, LeastConnections)

    // Simulate: backend 1 has 5 connections, backend 2 has 0
    backends := pool.GetBackends()
    for i := 0; i < 5; i++ {
        backends[0].IncrConnections()
    }

    b, _ := lb.Next()
    if b.URL.String() != "http://localhost:9002" {
        t.Errorf("should pick least-loaded backend (9002), got %s", b.URL.String())
    }
    t.Logf("✅ Least connections picked: %s (0 conns vs 5 conns)", b.URL.String())
}
