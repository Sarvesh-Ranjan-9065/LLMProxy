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

	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/authstore"
	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/cache"
	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/config"
	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/dashboard"
	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/middleware"
	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/ratelimit"
	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/router"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

	authStore, err := authstore.New(cfg.Auth, redisClient)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize auth store: %w", err)
	}

	// Initialize backend pool
	pool, err := router.NewPool(cfg.Workers, cfg.Router.StartBackendsAlive)
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
	reverseProxy := NewReverseProxy(lb, cfg)

	// Dashboard store (in-memory stats)
	dashboardStore := dashboard.NewStore(25)

	// ──────────────────────────────────────────────────────────────
	// Middleware chain — execution order (outermost → innermost):
	//
	//   Recovery  →  Auth  →  DashboardStats  →  Metrics  →  Logging  →  RateLimit  →  Cache  →  ReverseProxy
	//
	// • Recovery is outermost so panics anywhere are caught.
	// • Auth runs early so every subsequent middleware has api_key in context.
	// • Metrics / Logging now have accurate per-key attribution.
	// • RateLimit and Cache operate on authenticated requests only.
	// ──────────────────────────────────────────────────────────────
	handler := buildChain(
		reverseProxy,
		middleware.Recovery(),                                    // 1 — outermost
		middleware.Auth(authStore, cfg.Auth),                     // 2
		middleware.DashboardStats(dashboardStore),                // 3
		middleware.Metrics(),                                     // 4
		middleware.Logging(),                                     // 5
		middleware.RateLimit(tokenBucket, cfg.RateLimit),         // 6
		middleware.Cache(redisClient, hasher, ttlMgr, cfg.Cache), // 7 — innermost middleware
	)

	// Set up routes
	mux := http.NewServeMux()
	mux.Handle("/v1/chat/completions", handler)
	mux.Handle("/v1/completions", handler)
	mux.HandleFunc("/health", HealthHandler())

	adminOnly := func(h http.Handler) http.Handler {
		return buildChain(
			h,
			middleware.Recovery(),
			middleware.Auth(authStore, cfg.Auth),
			middleware.RequireRole("admin"),
		)
	}

	userOrAdmin := func(h http.Handler) http.Handler {
		return buildChain(
			h,
			middleware.Recovery(),
			middleware.Auth(authStore, cfg.Auth),
			middleware.RequireRole("user", "admin"),
		)
	}

	mux.Handle("/metrics", adminOnly(promhttp.Handler()))
	mux.Handle("/info", adminOnly(InfoHandler()))

	if cfg.Observability.PrometheusURL != "" {
		promProxy, err := newAdminReverseProxy(cfg.Observability.PrometheusURL, "/admin/prometheus")
		if err != nil {
			return nil, fmt.Errorf("failed to configure prometheus proxy: %w", err)
		}
		if promProxy != nil {
			mux.HandleFunc("/admin/prometheus", func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "/admin/prometheus/", http.StatusMovedPermanently)
			})
			mux.Handle("/admin/prometheus/", adminOnly(promProxy))
		}
	}

	if cfg.Observability.GrafanaURL != "" {
		grafProxy, err := newAdminReverseProxy(cfg.Observability.GrafanaURL, "/admin/grafana")
		if err != nil {
			return nil, fmt.Errorf("failed to configure grafana proxy: %w", err)
		}
		if grafProxy != nil {
			mux.HandleFunc("/admin/grafana", func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "/admin/grafana/", http.StatusMovedPermanently)
			})
			mux.Handle("/admin/grafana/", adminOnly(grafProxy))
		}
	}

	dashboardHandler := dashboard.NewHandler(dashboardStore, pool, redisClient)
	mux.Handle("/dashboard/api/me", userOrAdmin(http.HandlerFunc(dashboardHandler.Me)))
	mux.Handle("/dashboard/api/user/summary", userOrAdmin(http.HandlerFunc(dashboardHandler.UserSummary)))
	mux.Handle("/dashboard/api/admin/summary", adminOnly(http.HandlerFunc(dashboardHandler.AdminSummary)))

	mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard/", http.StatusMovedPermanently)
	})
	mux.Handle("/dashboard/", http.StripPrefix("/dashboard/", http.FileServer(http.Dir("dashboard"))))

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
