package ratelimit

import (
	"math"
	"sync"
	"time"
)

type localBucket struct {
	tokens   float64
	last     time.Time
	lastSeen time.Time
}

// InMemoryLimiter provides a local fallback when Redis is unavailable.
type InMemoryLimiter struct {
	mu              sync.Mutex
	buckets         map[string]*localBucket
	cleanupInterval time.Duration
	bucketTTL       time.Duration
	lastCleanup     time.Time
}

func NewInMemoryLimiter() *InMemoryLimiter {
	return &InMemoryLimiter{
		buckets:         make(map[string]*localBucket),
		cleanupInterval: time.Minute,
		bucketTTL:       15 * time.Minute,
		lastCleanup:     time.Now(),
	}
}

// Allow applies a local token bucket. Returns allowed, remaining, retryAfter.
func (l *InMemoryLimiter) Allow(key string, rate float64, burst int) (bool, int, time.Duration) {
	if rate <= 0 || burst <= 0 {
		return false, 0, time.Second
	}

	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	l.cleanup(now)

	bucket := l.buckets[key]
	if bucket == nil {
		bucket = &localBucket{
			tokens:   float64(burst),
			last:     now,
			lastSeen: now,
		}
		l.buckets[key] = bucket
	}

	delta := now.Sub(bucket.last).Seconds()
	if delta < 0 {
		delta = 0
	}
	bucket.tokens = math.Min(float64(burst), bucket.tokens+(delta*rate))
	bucket.last = now
	bucket.lastSeen = now

	allowed := bucket.tokens >= 1
	if allowed {
		bucket.tokens -= 1
	}

	remaining := int(math.Floor(bucket.tokens))
	if remaining < 0 {
		remaining = 0
	}

	retryAfter := time.Duration(0)
	if !allowed {
		needed := 1 - bucket.tokens
		if needed < 0 {
			needed = 0
		}
		retryAfterSeconds := needed / rate
		retryAfter = time.Duration(math.Ceil(retryAfterSeconds * float64(time.Second)))
		if retryAfter < 0 {
			retryAfter = 0
		}
	}

	return allowed, remaining, retryAfter
}

func (l *InMemoryLimiter) cleanup(now time.Time) {
	if now.Sub(l.lastCleanup) < l.cleanupInterval {
		return
	}
	for key, bucket := range l.buckets {
		if now.Sub(bucket.lastSeen) > l.bucketTTL {
			delete(l.buckets, key)
		}
	}
	l.lastCleanup = now
}
