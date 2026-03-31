package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/yourusername/llmproxy/internal/cache"
	"github.com/yourusername/llmproxy/internal/config"
	"github.com/yourusername/llmproxy/internal/middleware"
	"github.com/yourusername/llmproxy/internal/ratelimit"
	"github.com/yourusername/llmproxy/internal/router"
)

type Server struct {
	cfg           *config.Config
	httpServer    *http.Server
	healthChecker *router.HealthChecker
	redisClient   *cache.RedisClient
}

func NewServer(cfg *config.Config) (*Server, error) {
	// Initialize Redis
	redisClient, err := cache.NewRedisClient(cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}
	slog.Info("connected to Redis", "addr", cfg.Redis.Addr)

	// Initialize backend pool
	pool, err := router.NewPool(cfg.Workers)
	if err != nil {
		return nil, fmt.Errorf("failed to create backend pool: %w", err)
	}

	// Initialize load balancer (round-robin)
	lb := router.NewLoadBalancer(pool, router.RoundRobin)

	// Initialize health checker
	healthChecker := router.NewHealthChecker(pool, 10*time.Second, 3*time.Second)

	// Initialize rate limiter
	store := ratelimit.NewStore(redisClient)
	tokenBucket := ratelimit.NewTokenBucket(store)

	// Initialize cache components
	hasher := cache.NewSemanticHasher()
	ttlMgr := cache.NewTTLManager(redisClient, cfg.Cache.TTL)

	// Initialize reverse proxy
	reverseProxy := NewReverseProxy(lb)

	// ──────────────────────────────────────────────────────────────
	// Middleware chain — execution order (outermost → innermost):
	//
	//   Recovery  →  Auth  →  Metrics  →  Logging  →  RateLimit  →  Cache  →  ReverseProxy
	//
	// • Recovery is outermost so panics anywhere are caught.
	// • Auth runs early so every subsequent middleware has api_key in context.
	// • Metrics / Logging now have accurate per-key attribution.
	// • RateLimit and Cache operate on authenticated requests only.
	// ──────────────────────────────────────────────────────────────
	handler := buildChain(
		reverseProxy,
		middleware.Recovery(),                                      // 1 — outermost
		middleware.Auth(cfg.Auth),                                  // 2
		middleware.Metrics(),                                       // 3
		middleware.Logging(),                                       // 4
		middleware.RateLimit(tokenBucket, cfg.RateLimit),           // 5
		middleware.Cache(redisClient, hasher, ttlMgr, cfg.Cache),  // 6 — innermost middleware
	)

	// Set up routes
	mux := http.NewServeMux()
	mux.Handle("/v1/chat/completions", handler)
	mux.Handle("/v1/completions", handler)
	mux.HandleFunc("/health", HealthHandler())
	mux.HandleFunc("/info", InfoHandler())
	mux.Handle("/metrics", promhttp.Handler())

	httpServer := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	return &Server{
		cfg:           cfg,
		httpServer:    httpServer,
		healthChecker: healthChecker,
		redisClient:   redisClient,
	}, nil
}

func (s *Server) Start() error {
	// Start health checker
	ctx := context.Background()
	s.healthChecker.Start(ctx)

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		slog.Info("shutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), s.cfg.Server.ShutdownTimeout)
		defer cancel()

		s.healthChecker.Stop()
		s.redisClient.Close()
		s.httpServer.Shutdown(ctx)
	}()

	slog.Info("LLMProxy gateway starting",
		"port", s.cfg.Server.Port,
		"workers", len(s.cfg.Workers),
	)

	if err := s.httpServer.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// buildChain applies middleware in order (first middleware = outermost)
func buildChain(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}