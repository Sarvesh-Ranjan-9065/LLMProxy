package metrics

import (
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ─── Request metrics ───────────────────────────────────────
	// By default we do NOT include raw API keys as labels to avoid
	// high-cardinality label explosions in Prometheus. Operators can
	// enable per-key metrics via the `ENABLE_PER_KEY_METRICS` env var.
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llmproxy_requests_total",
			Help: "Total number of requests processed",
		},
		[]string{"method", "path", "status"},
	)

	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "llmproxy_request_duration_seconds",
			Help:    "Request duration in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"method", "path", "status"},
	)

	// ─── Cache metrics ─────────────────────────────────────────
	CacheHitsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "llmproxy_cache_hits_total",
			Help: "Total number of cache hits",
		},
	)

	CacheMissesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "llmproxy_cache_misses_total",
			Help: "Total number of cache misses",
		},
	)
	// NOTE: cache hit *rate* is intentionally NOT a gauge here.
	// Derive it in PromQL / Grafana:
	//   llmproxy_cache_hits_total / (llmproxy_cache_hits_total + llmproxy_cache_misses_total)

	// ─── Auth metrics ──────────────────────────────────────────
	AuthRejectedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llmproxy_auth_rejected_total",
			Help: "Total authentication rejections (before main metrics middleware)",
		},
		[]string{"reason"},
	)

	// ─── Rate limit metrics ────────────────────────────────────
	RateLimitedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llmproxy_rate_limited_total",
			Help: "Total number of rate-limited requests",
		},
		[]string{},
	)

	// ─── Token usage metrics ───────────────────────────────────
	TokensUsed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llmproxy_tokens_used_total",
			Help: "Total tokens used (estimated)",
		},
		[]string{"type"},
	)

	EstimatedCost = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llmproxy_estimated_cost_dollars",
			Help: "Estimated cost in dollars",
		},
		[]string{},
	)

	// ─── Backend metrics ───────────────────────────────────────
	BackendRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llmproxy_backend_requests_total",
			Help: "Total requests sent to backend workers",
		},
		[]string{"backend", "status"},
	)

	BackendLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "llmproxy_backend_latency_seconds",
			Help:    "Backend worker latency",
			Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 3, 5, 10},
		},
		[]string{"backend"},
	)

	// ─── Connection metrics ────────────────────────────────────
	ActiveConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "llmproxy_active_connections",
			Help: "Number of active connections",
		},
	)

	WorkerHealthStatus = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "llmproxy_worker_health",
			Help: "Worker health status (1=healthy, 0=unhealthy)",
		},
		[]string{"worker"},
	)

	// Optional per-key metrics (created when ENABLE_PER_KEY_METRICS=1)
	PerKeyRequestsTotal *prometheus.CounterVec
	PerKeyEnabledGauge  prometheus.Gauge
)

func init() {
	if os.Getenv("ENABLE_PER_KEY_METRICS") == "1" || os.Getenv("ENABLE_PER_KEY_METRICS") == "true" {
		// create a simple gauge so operators can tell per-key metrics are enabled
		PerKeyEnabledGauge = promauto.NewGauge(prometheus.GaugeOpts{
			Name: "llmproxy_per_key_metrics_enabled",
			Help: "Whether per-key metrics are enabled (1=true)",
		})
		PerKeyEnabledGauge.Set(1)
		PerKeyRequestsTotal = promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "llmproxy_requests_per_key_total",
				Help: "Total number of requests broken down by api_key (opt-in)",
			},
			[]string{"method", "path", "status", "api_key"},
		)
	} else {
		// ensure gauge exists but is zero
		PerKeyEnabledGauge = promauto.NewGauge(prometheus.GaugeOpts{
			Name: "llmproxy_per_key_metrics_enabled",
			Help: "Whether per-key metrics are enabled (1=true)",
		})
		PerKeyEnabledGauge.Set(0)
	}
}
