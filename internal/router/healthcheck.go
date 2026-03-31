package router

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/yourusername/llmproxy/internal/metrics"
)

// HealthChecker periodically checks backend health
type HealthChecker struct {
	pool     *Pool
	interval time.Duration
	timeout  time.Duration
	client   *http.Client
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func NewHealthChecker(pool *Pool, interval, timeout time.Duration) *HealthChecker {
	return &HealthChecker{
		pool:     pool,
		interval: interval,
		timeout:  timeout,
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout: timeout,
				}).DialContext,
			},
		},
	}
}

func (hc *HealthChecker) Start(ctx context.Context) {
	ctx, hc.cancel = context.WithCancel(ctx)
	hc.wg.Add(1)

	go func() {
		defer hc.wg.Done()
		ticker := time.NewTicker(hc.interval)
		defer ticker.Stop()

		// Initial check
		hc.checkAll(ctx)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				hc.checkAll(ctx)
			}
		}
	}()
}

func (hc *HealthChecker) Stop() {
	if hc.cancel != nil {
		hc.cancel()
	}
	hc.wg.Wait()
}

func (hc *HealthChecker) checkAll(ctx context.Context) {
	backends := hc.pool.GetBackends()
	var wg sync.WaitGroup

	for _, backend := range backends {
		wg.Add(1)
		go func(b *Backend) {
			defer wg.Done()
			alive := hc.checkBackend(ctx, b)
			wasAlive := b.IsAlive()
			b.SetAlive(alive)

			// Update Prometheus metric
			healthVal := 0.0
			if alive {
				healthVal = 1.0
			}
			metrics.WorkerHealthStatus.WithLabelValues(b.URL.String()).Set(healthVal)

			if wasAlive && !alive {
				slog.Warn("Backend went DOWN",
					"url", b.URL.String(),
				)
			} else if !wasAlive && alive {
				slog.Info("Backend came UP",
					"url", b.URL.String(),
				)
			}
		}(backend)
	}

	wg.Wait()
}

func (hc *HealthChecker) checkBackend(ctx context.Context, b *Backend) bool {
	healthURL := b.URL.String() + "/health"

	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		return false
	}

	resp, err := hc.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}