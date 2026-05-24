package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	internalCache "github.com/Sarvesh-Ranjan-9065/llmproxy/internal/cache"
	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/config"
)

type fakeCacheStore struct{}

func (f *fakeCacheStore) Get(ctx context.Context, key string) (string, error) {
	return "", nil
}

type fakeTTLManager struct {
	sleep   time.Duration
	active  int32
	max     int32
	started int32
}

func (f *fakeTTLManager) SetWithTTL(ctx context.Context, key string, value string, ttl time.Duration) error {
	atomic.AddInt32(&f.started, 1)
	current := atomic.AddInt32(&f.active, 1)
	for {
		max := atomic.LoadInt32(&f.max)
		if current <= max {
			break
		}
		if atomic.CompareAndSwapInt32(&f.max, max, current) {
			break
		}
	}

	time.Sleep(f.sleep)

	atomic.AddInt32(&f.active, -1)
	return nil
}

func (f *fakeTTLManager) maxConcurrent() int32 {
	return atomic.LoadInt32(&f.max)
}

func TestCacheWriteConcurrencyBounded(t *testing.T) {
	store := &fakeCacheStore{}
	writer := &fakeTTLManager{sleep: 10 * time.Millisecond}
	cfg := config.CacheConfig{Enabled: true, TTL: 5 * time.Minute}

	hasher := internalCache.NewSemanticHasher()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	handler := Cache(store, hasher, writer, cfg)(next)

	const requests = 200

	var reqWG sync.WaitGroup
	reqWG.Add(requests)

	body := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"hi"}]}`
	for i := 0; i < requests; i++ {
		go func() {
			defer reqWG.Done()
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", rec.Code)
			}
		}()
	}

	reqWG.Wait()

	deadline := time.Now().Add(3 * time.Second)
	for {
		if atomic.LoadInt32(&writer.active) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for cache writes")
		}
		time.Sleep(5 * time.Millisecond)
	}

	if atomic.LoadInt32(&writer.started) == 0 {
		t.Fatal("expected cache writes to start")
	}

	if max := writer.maxConcurrent(); max > 50 {
		t.Fatalf("expected cache write concurrency <= 50, got %d", max)
	}
}
