package router

import (
	"net/url"
	"sync"

	"github.com/yourusername/llmproxy/internal/config"
)

// Backend represents a single backend worker
type Backend struct {
	URL               *url.URL
	Alive             bool
	Weight            int
	ActiveConnections int64
	mu                sync.RWMutex
}

func (b *Backend) SetAlive(alive bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Alive = alive
}

func (b *Backend) IsAlive() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.Alive
}

func (b *Backend) IncrConnections() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ActiveConnections++
}

func (b *Backend) DecrConnections() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.ActiveConnections > 0 {
		b.ActiveConnections--
	}
}

func (b *Backend) GetConnections() int64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.ActiveConnections
}

// Pool manages a pool of backend workers
type Pool struct {
	backends []*Backend
	mu       sync.RWMutex
}

func NewPool(workers []config.WorkerConfig) (*Pool, error) {
	pool := &Pool{}

	for _, w := range workers {
		u, err := url.Parse(w.URL)
		if err != nil {
			return nil, err
		}
		pool.backends = append(pool.backends, &Backend{
			URL:    u,
			Alive:  true,
			Weight: w.Weight,
		})
	}

	return pool, nil
}

// GetBackends returns a shallow copy of the backend slice so callers
// cannot accidentally modify the internal pool state.
func (p *Pool) GetBackends() []*Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]*Backend, len(p.backends))
	copy(out, p.backends)
	return out
}

// GetAliveBackends returns a new slice containing only healthy backends.
func (p *Pool) GetAliveBackends() []*Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()

	alive := make([]*Backend, 0, len(p.backends))
	for _, b := range p.backends {
		if b.IsAlive() {
			alive = append(alive, b)
		}
	}
	return alive
}

func (p *Pool) AddBackend(b *Backend) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.backends = append(p.backends, b)
}