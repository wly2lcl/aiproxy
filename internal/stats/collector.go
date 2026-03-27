package stats

import (
	"sort"
	"sync"
	"time"

	"github.com/wangluyao/aiproxy/internal/domain"
)

type RequestMetrics struct {
	Count   int64
	Errors  int64
	Tokens  int64
	Latency []time.Duration
}

type ErrorMetric struct {
	Provider  string
	Model     string
	ErrorType string
	Count     int64
}

type RateLimitMetric struct {
	AccountID string
	LimitType domain.LimitType
	Count     int64
}

type Snapshot struct {
	Requests     map[string]*RequestMetrics
	Errors       []ErrorMetric
	RateLimits   []RateLimitMetric
	TotalTokens  int64
	TotalErrors  int64
	TotalLatency time.Duration
	LatencyCount int64
}

type Collector struct {
	mu sync.RWMutex

	requests   map[string]*RequestMetrics
	errors     map[string]int64
	rateLimits map[string]int64

	totalTokens  int64
	totalErrors  int64
	totalLatency time.Duration
	latencyCount int64
}

func NewCollector() *Collector {
	return &Collector{
		requests:   make(map[string]*RequestMetrics),
		errors:     make(map[string]int64),
		rateLimits: make(map[string]int64),
	}
}

func (c *Collector) RecordRequest(provider, model string, status int, latency time.Duration, tokens int) {
	key := provider + ":" + model

	c.mu.Lock()
	defer c.mu.Unlock()

	metrics, ok := c.requests[key]
	if !ok {
		metrics = &RequestMetrics{
			Latency: make([]time.Duration, 0, 1000),
		}
		c.requests[key] = metrics
	}

	metrics.Count++
	metrics.Tokens += int64(tokens)
	metrics.Latency = append(metrics.Latency, latency)

	c.totalTokens += int64(tokens)
	c.totalLatency += latency
	c.latencyCount++

	if status >= 400 {
		metrics.Errors++
		c.totalErrors++
	}
}

func (c *Collector) RecordError(provider, model string, errorType string) {
	key := provider + ":" + model + ":" + errorType

	c.mu.Lock()
	defer c.mu.Unlock()

	c.errors[key]++
	c.totalErrors++
}

func (c *Collector) RecordRateLimitHit(accountID string, limitType domain.LimitType) {
	key := accountID + ":" + string(limitType)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.rateLimits[key]++
}

func (c *Collector) GetSnapshot() *Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	snapshot := &Snapshot{
		Requests:     make(map[string]*RequestMetrics),
		Errors:       make([]ErrorMetric, 0, len(c.errors)),
		RateLimits:   make([]RateLimitMetric, 0, len(c.rateLimits)),
		TotalTokens:  c.totalTokens,
		TotalErrors:  c.totalErrors,
		TotalLatency: c.totalLatency,
		LatencyCount: c.latencyCount,
	}

	for key, metrics := range c.requests {
		copied := *metrics
		copied.Latency = make([]time.Duration, len(metrics.Latency))
		copy(copied.Latency, metrics.Latency)
		snapshot.Requests[key] = &copied

		// FIX: Clear the latency slice after snapshot to prevent memory leak (OOM)
		// We retain the slice capacity for GC efficiency
		metrics.Latency = metrics.Latency[:0]
	}

	for key, count := range c.errors {
		parts := splitKey(key, 3)
		if len(parts) == 3 {
			snapshot.Errors = append(snapshot.Errors, ErrorMetric{
				Provider:  parts[0],
				Model:     parts[1],
				ErrorType: parts[2],
				Count:     count,
			})
		}
	}

	for key, count := range c.rateLimits {
		parts := splitKey(key, 2)
		if len(parts) == 2 {
			snapshot.RateLimits = append(snapshot.RateLimits, RateLimitMetric{
				AccountID: parts[0],
				LimitType: domain.LimitType(parts[1]),
				Count:     count,
			})
		}
	}

	return snapshot
}

func (s *Snapshot) LatencyPercentiles() (p50, p95, p99 time.Duration) {
	var allLatencies []time.Duration
	for _, m := range s.Requests {
		allLatencies = append(allLatencies, m.Latency...)
	}

	if len(allLatencies) == 0 {
		return 0, 0, 0
	}

	sort.Slice(allLatencies, func(i, j int) bool {
		return allLatencies[i] < allLatencies[j]
	})

	n := len(allLatencies)
	p50 = allLatencies[n*50/100]
	p95 = allLatencies[n*95/100]
	p99 = allLatencies[n*99/100]

	return
}

func (s *Snapshot) RequestsByProvider() map[string]int64 {
	result := make(map[string]int64)
	for key, m := range s.Requests {
		parts := splitKey(key, 2)
		if len(parts) >= 1 {
			result[parts[0]] += m.Count
		}
	}
	return result
}

func (s *Snapshot) RequestsByModel() map[string]int64 {
	result := make(map[string]int64)
	for key, m := range s.Requests {
		parts := splitKey(key, 2)
		if len(parts) >= 2 {
			result[parts[1]] += m.Count
		}
	}
	return result
}

func (s *Snapshot) TokensByProvider() map[string]int64 {
	result := make(map[string]int64)
	for key, m := range s.Requests {
		parts := splitKey(key, 2)
		if len(parts) >= 1 {
			result[parts[0]] += m.Tokens
		}
	}
	return result
}

func (s *Snapshot) TokensByModel() map[string]int64 {
	result := make(map[string]int64)
	for key, m := range s.Requests {
		parts := splitKey(key, 2)
		if len(parts) >= 2 {
			result[parts[1]] += m.Tokens
		}
	}
	return result
}

func splitKey(key string, expectedParts int) []string {
	result := make([]string, 0, expectedParts)
	start := 0
	for i := 0; i < len(key) && len(result) < expectedParts-1; i++ {
		if key[i] == ':' {
			result = append(result, key[start:i])
			start = i + 1
		}
	}
	if start < len(key) {
		result = append(result, key[start:])
	}
	return result
}
