package proxy

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/metrics"
	"github.com/Sarvesh-Ranjan-9065/llmproxy/internal/router"
)

// ReverseProxy forwards requests to backend workers
type ReverseProxy struct {
	lb     *router.LoadBalancer
	client *http.Client
}

func NewReverseProxy(lb *router.LoadBalancer) *ReverseProxy {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}

	return &ReverseProxy{
		lb: lb,
		client: &http.Client{
			Transport: transport,
			Timeout:   60 * time.Second,
		},
	}
}

func (rp *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Select a backend
	backend, err := rp.lb.Next()
	if err != nil {
		slog.Error("no backends available", "error", err)
		http.Error(w, `{"error":{"message":"no backends available","type":"server_error","code":503}}`,
			http.StatusServiceUnavailable)
		return
	}

	backend.IncrConnections()
	defer backend.DecrConnections()

	start := time.Now()

	// Build full target URL preserving path AND query string
	targetURL := backend.URL.String() + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		slog.Error("failed to create proxy request", "error", err)
		http.Error(w, `{"error":{"message":"proxy error","type":"server_error","code":502}}`,
			http.StatusBadGateway)
		return
	}

	// Copy headers from original request
	for key, values := range r.Header {
		for _, v := range values {
			proxyReq.Header.Add(key, v)
		}
	}

	// Correct X-Forwarded-For: strip port from RemoteAddr and append
	clientIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(clientIP); err == nil {
		clientIP = host
	}
	if prior := r.Header.Get("X-Forwarded-For"); prior != "" {
		proxyReq.Header.Set("X-Forwarded-For", prior+", "+clientIP)
	} else {
		proxyReq.Header.Set("X-Forwarded-For", clientIP)
	}
	proxyReq.Header.Set("X-Forwarded-Host", r.Host)

	// Send request to backend
	resp, err := rp.client.Do(proxyReq)
	if err != nil {
		slog.Error("backend request failed",
			"backend", backend.URL.String(),
			"error", err,
		)
		metrics.BackendRequestsTotal.WithLabelValues(backend.URL.String(), "error").Inc()
		http.Error(w, `{"error":{"message":"backend unavailable","type":"server_error","code":502}}`,
			http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	duration := time.Since(start).Seconds()
	metrics.BackendLatency.WithLabelValues(backend.URL.String()).Observe(duration)
	metrics.BackendRequestsTotal.WithLabelValues(backend.URL.String(), "success").Inc()

	// Copy response headers
	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	w.Header().Set("X-Backend", backend.URL.String())

	// Write status and body
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		slog.Error("failed to write backend response body",
			"error", err,
			"backend", backend.URL.String(),
		)
	}
}