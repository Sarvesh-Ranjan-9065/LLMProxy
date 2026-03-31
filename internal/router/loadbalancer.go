package router

import (
	"errors"
	"sync/atomic"
)

type Strategy int

const (
	RoundRobin Strategy = iota
	Weighted
	LeastConnections
)

var ErrNoBackendsAvailable = errors.New("no healthy backends available")

// LoadBalancer selects backend workers using different strategies
type LoadBalancer struct {
	pool     *Pool
	strategy Strategy
	current  uint64
}

func NewLoadBalancer(pool *Pool, strategy Strategy) *LoadBalancer {
	return &LoadBalancer{
		pool:     pool,
		strategy: strategy,
	}
}

// Next returns the next available backend based on the configured strategy
func (lb *LoadBalancer) Next() (*Backend, error) {
	switch lb.strategy {
	case RoundRobin:
		return lb.roundRobin()
	case Weighted:
		return lb.weighted()
	case LeastConnections:
		return lb.leastConnections()
	default:
		return lb.roundRobin()
	}
}

func (lb *LoadBalancer) roundRobin() (*Backend, error) {
	backends := lb.pool.GetAliveBackends()
	if len(backends) == 0 {
		return nil, ErrNoBackendsAvailable
	}

	next := atomic.AddUint64(&lb.current, 1)
	idx := int(next) % len(backends)
	return backends[idx], nil
}

func (lb *LoadBalancer) weighted() (*Backend, error) {
	backends := lb.pool.GetAliveBackends()
	if len(backends) == 0 {
		return nil, ErrNoBackendsAvailable
	}

	// Sum weights — guard against all-zero configuration
	totalWeight := 0
	for _, b := range backends {
		totalWeight += b.Weight
	}

	if totalWeight <= 0 {
		// Every alive backend has weight 0 — fall back to round-robin
		// so we never divide/modulo by zero.
		return lb.roundRobin()
	}

	next := int(atomic.AddUint64(&lb.current, 1)) % totalWeight
	for _, b := range backends {
		next -= b.Weight
		if next < 0 {
			return b, nil
		}
	}

	// Should never reach here, but be safe
	return backends[0], nil
}

func (lb *LoadBalancer) leastConnections() (*Backend, error) {
	backends := lb.pool.GetAliveBackends()
	if len(backends) == 0 {
		return nil, ErrNoBackendsAvailable
	}

	var least *Backend
	var minConn int64 = -1

	for _, b := range backends {
		conns := b.GetConnections()
		if minConn == -1 || conns < minConn {
			least = b
			minConn = conns
		}
	}

	return least, nil
}

func (lb *LoadBalancer) Pool() *Pool {
	return lb.pool
}