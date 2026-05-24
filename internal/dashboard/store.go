package dashboard

import (
	"sort"
	"sync"
	"time"
)

type RequestInfo struct {
	APIKey    string
	Owner     string
	Role      string
	Tier      string
	Method    string
	Path      string
	Status    int
	LatencyMs float64
	Cache     string
	Model     string
	When      time.Time
}

type RequestEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	Status    int       `json:"status"`
	LatencyMs float64   `json:"latency_ms"`
	Cache     string    `json:"cache"`
	Model     string    `json:"model"`
}

type UserSummary struct {
	Owner         string            `json:"owner"`
	Role          string            `json:"role"`
	Tier          string            `json:"tier,omitempty"`
	RequestsTotal uint64            `json:"requests_total"`
	AvgLatencyMs  float64           `json:"avg_latency_ms"`
	CacheHitRate  float64           `json:"cache_hit_rate"`
	CacheHits     uint64            `json:"cache_hits"`
	CacheMisses   uint64            `json:"cache_misses"`
	RateLimited   uint64            `json:"rate_limited_total"`
	ModelUsage    map[string]uint64 `json:"model_usage"`
	Recent        []RequestEvent    `json:"recent"`
	GeneratedAt   time.Time         `json:"generated_at"`
}

type BackendStatus struct {
	URL               string `json:"url"`
	Alive             bool   `json:"alive"`
	ActiveConnections int64  `json:"active_connections"`
}

type RedisStatus struct {
	Healthy bool   `json:"healthy"`
	Error   string `json:"error,omitempty"`
}

type AdminSummary struct {
	TotalRequests   uint64            `json:"total_requests"`
	ActiveUsers     int               `json:"active_users"`
	AvgLatencyMs    float64           `json:"avg_latency_ms"`
	CacheHitRate    float64           `json:"cache_hit_rate"`
	CacheHits       uint64            `json:"cache_hits"`
	CacheMisses     uint64            `json:"cache_misses"`
	RateLimited     uint64            `json:"rate_limited_total"`
	ModelUsage      map[string]uint64 `json:"model_usage"`
	Backends        []BackendStatus   `json:"backends"`
	Redis           RedisStatus       `json:"redis"`
	GeneratedAt     time.Time         `json:"generated_at"`
	RecentRequests  []RequestEvent    `json:"recent_requests"`
	TopModels       []string          `json:"top_models"`
	TopModelCounts  []uint64          `json:"top_model_counts"`
	TopModelMaxSize int               `json:"top_model_max_size"`
}

type keyStats struct {
	Owner         string
	Role          string
	Tier          string
	RequestsTotal uint64
	LatencyTotal  float64
	CacheHits     uint64
	CacheMisses   uint64
	RateLimited   uint64
	ModelUsage    map[string]uint64
	Recent        []RequestEvent
}

type Store struct {
	mu        sync.RWMutex
	maxRecent int
	perKey    map[string]*keyStats

	totalRequests  uint64
	latencyTotal   float64
	cacheHits      uint64
	cacheMisses    uint64
	rateLimited    uint64
	modelUsage     map[string]uint64
	recentRequests []RequestEvent
}

func NewStore(maxRecent int) *Store {
	if maxRecent <= 0 {
		maxRecent = 20
	}
	return &Store{
		maxRecent:  maxRecent,
		perKey:     make(map[string]*keyStats),
		modelUsage: make(map[string]uint64),
	}
}

func (s *Store) Record(info RequestInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stats := s.perKey[info.APIKey]
	if stats == nil {
		stats = &keyStats{
			Owner:      info.Owner,
			Role:       info.Role,
			Tier:       info.Tier,
			ModelUsage: make(map[string]uint64),
		}
		s.perKey[info.APIKey] = stats
	}

	stats.Owner = info.Owner
	stats.Role = info.Role
	stats.Tier = info.Tier
	stats.RequestsTotal++
	stats.LatencyTotal += info.LatencyMs

	s.totalRequests++
	s.latencyTotal += info.LatencyMs

	if info.Status == 429 {
		stats.RateLimited++
		s.rateLimited++
	}

	switch info.Cache {
	case "HIT":
		stats.CacheHits++
		s.cacheHits++
	case "MISS":
		stats.CacheMisses++
		s.cacheMisses++
	}

	if info.Model != "" {
		stats.ModelUsage[info.Model]++
		s.modelUsage[info.Model]++
	}

	event := RequestEvent{
		Timestamp: info.When,
		Method:    info.Method,
		Path:      info.Path,
		Status:    info.Status,
		LatencyMs: info.LatencyMs,
		Cache:     info.Cache,
		Model:     info.Model,
	}

	stats.Recent = append(stats.Recent, event)
	if len(stats.Recent) > s.maxRecent {
		stats.Recent = stats.Recent[len(stats.Recent)-s.maxRecent:]
	}

	s.recentRequests = append(s.recentRequests, event)
	if len(s.recentRequests) > s.maxRecent {
		s.recentRequests = s.recentRequests[len(s.recentRequests)-s.maxRecent:]
	}
}

func (s *Store) GetUserSummary(apiKey string) UserSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.perKey[apiKey]
	if stats == nil {
		return UserSummary{GeneratedAt: time.Now()}
	}

	avgLatency := 0.0
	if stats.RequestsTotal > 0 {
		avgLatency = stats.LatencyTotal / float64(stats.RequestsTotal)
	}

	cacheHitRate := 0.0
	if stats.CacheHits+stats.CacheMisses > 0 {
		cacheHitRate = float64(stats.CacheHits) / float64(stats.CacheHits+stats.CacheMisses)
	}

	return UserSummary{
		Owner:         stats.Owner,
		Role:          stats.Role,
		Tier:          stats.Tier,
		RequestsTotal: stats.RequestsTotal,
		AvgLatencyMs:  avgLatency,
		CacheHitRate:  cacheHitRate,
		CacheHits:     stats.CacheHits,
		CacheMisses:   stats.CacheMisses,
		RateLimited:   stats.RateLimited,
		ModelUsage:    copyMap(stats.ModelUsage),
		Recent:        copyEvents(stats.Recent),
		GeneratedAt:   time.Now(),
	}
}

func (s *Store) GetAdminSummary(topModelLimit int) AdminSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	avgLatency := 0.0
	if s.totalRequests > 0 {
		avgLatency = s.latencyTotal / float64(s.totalRequests)
	}

	cacheHitRate := 0.0
	if s.cacheHits+s.cacheMisses > 0 {
		cacheHitRate = float64(s.cacheHits) / float64(s.cacheHits+s.cacheMisses)
	}

	activeUsers := 0
	for _, stats := range s.perKey {
		if stats.RequestsTotal > 0 {
			activeUsers++
		}
	}

	modelUsage := copyMap(s.modelUsage)
	modelNames, modelCounts := topModels(modelUsage, topModelLimit)

	return AdminSummary{
		TotalRequests:   s.totalRequests,
		ActiveUsers:     activeUsers,
		AvgLatencyMs:    avgLatency,
		CacheHitRate:    cacheHitRate,
		CacheHits:       s.cacheHits,
		CacheMisses:     s.cacheMisses,
		RateLimited:     s.rateLimited,
		ModelUsage:      modelUsage,
		GeneratedAt:     time.Now(),
		RecentRequests:  copyEvents(s.recentRequests),
		TopModels:       modelNames,
		TopModelCounts:  modelCounts,
		TopModelMaxSize: topModelLimit,
	}
}

func copyMap(src map[string]uint64) map[string]uint64 {
	if len(src) == 0 {
		return map[string]uint64{}
	}
	out := make(map[string]uint64, len(src))
	for key, val := range src {
		out[key] = val
	}
	return out
}

func copyEvents(src []RequestEvent) []RequestEvent {
	if len(src) == 0 {
		return []RequestEvent{}
	}
	out := make([]RequestEvent, len(src))
	copy(out, src)
	return out
}

func topModels(models map[string]uint64, limit int) ([]string, []uint64) {
	if limit <= 0 {
		limit = 5
	}

	type kv struct {
		key   string
		value uint64
	}
	items := make([]kv, 0, len(models))
	for key, value := range models {
		items = append(items, kv{key: key, value: value})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].value > items[j].value
	})

	if len(items) > limit {
		items = items[:limit]
	}

	names := make([]string, len(items))
	counts := make([]uint64, len(items))
	for i, item := range items {
		names[i] = item.key
		counts[i] = item.value
	}

	return names, counts
}
