package stats

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangluyao/aiproxy/internal/domain"
)

func TestCollector_RecordRequest(t *testing.T) {
	c := NewCollector()

	c.RecordRequest("openai", "gpt-4", 200, 100*time.Millisecond, 100)
	c.RecordRequest("openai", "gpt-4", 200, 150*time.Millisecond, 150)
	c.RecordRequest("openai", "gpt-3.5-turbo", 200, 50*time.Millisecond, 50)

	snapshot := c.GetSnapshot()

	if len(snapshot.Requests) != 2 {
		t.Errorf("expected 2 request keys, got %d", len(snapshot.Requests))
	}

	key := "openai:gpt-4"
	m, ok := snapshot.Requests[key]
	if !ok {
		t.Fatalf("expected request metrics for %s", key)
	}

	if m.Count != 2 {
		t.Errorf("expected count 2, got %d", m.Count)
	}

	if m.Tokens != 250 {
		t.Errorf("expected tokens 250, got %d", m.Tokens)
	}

	if len(m.Latency) != 2 {
		t.Errorf("expected 2 latency entries, got %d", len(m.Latency))
	}
}

func TestCollector_RecordRequest_WithErrors(t *testing.T) {
	c := NewCollector()

	c.RecordRequest("openai", "gpt-4", 200, 100*time.Millisecond, 100)
	c.RecordRequest("openai", "gpt-4", 500, 200*time.Millisecond, 0)
	c.RecordRequest("openai", "gpt-4", 429, 50*time.Millisecond, 0)

	snapshot := c.GetSnapshot()

	key := "openai:gpt-4"
	m, ok := snapshot.Requests[key]
	if !ok {
		t.Fatalf("expected request metrics for %s", key)
	}

	if m.Count != 3 {
		t.Errorf("expected count 3, got %d", m.Count)
	}

	if m.Errors != 2 {
		t.Errorf("expected errors 2, got %d", m.Errors)
	}

	if snapshot.TotalErrors != 2 {
		t.Errorf("expected total errors 2, got %d", snapshot.TotalErrors)
	}
}

func TestCollector_RecordError(t *testing.T) {
	c := NewCollector()

	c.RecordError("openai", "gpt-4", "timeout")
	c.RecordError("openai", "gpt-4", "timeout")
	c.RecordError("openai", "gpt-4", "rate_limit")
	c.RecordError("groq", "llama-2", "timeout")

	snapshot := c.GetSnapshot()

	if len(snapshot.Errors) != 3 {
		t.Errorf("expected 3 error entries, got %d", len(snapshot.Errors))
	}

	var timeoutCount int64
	for _, e := range snapshot.Errors {
		if e.ErrorType == "timeout" {
			timeoutCount += e.Count
		}
	}

	if timeoutCount != 3 {
		t.Errorf("expected timeout count 3, got %d", timeoutCount)
	}
}

func TestCollector_RecordRateLimitHit(t *testing.T) {
	c := NewCollector()

	c.RecordRateLimitHit("acc-1", domain.LimitTypeRPM)
	c.RecordRateLimitHit("acc-1", domain.LimitTypeRPM)
	c.RecordRateLimitHit("acc-1", domain.LimitTypeDaily)
	c.RecordRateLimitHit("acc-2", domain.LimitTypeRPM)

	snapshot := c.GetSnapshot()

	if len(snapshot.RateLimits) != 3 {
		t.Errorf("expected 3 rate limit entries, got %d", len(snapshot.RateLimits))
	}

	var rpmCount int64
	for _, rl := range snapshot.RateLimits {
		if rl.LimitType == domain.LimitTypeRPM {
			rpmCount += rl.Count
		}
	}

	if rpmCount != 3 {
		t.Errorf("expected RPM count 3, got %d", rpmCount)
	}
}

func TestCollector_GetSnapshot(t *testing.T) {
	c := NewCollector()

	c.RecordRequest("openai", "gpt-4", 200, 100*time.Millisecond, 100)
	c.RecordError("openai", "gpt-4", "timeout")
	c.RecordRateLimitHit("acc-1", domain.LimitTypeRPM)

	snapshot := c.GetSnapshot()

	if snapshot.TotalTokens != 100 {
		t.Errorf("expected total tokens 100, got %d", snapshot.TotalTokens)
	}

	if snapshot.TotalErrors != 1 {
		t.Errorf("expected total errors 1, got %d", snapshot.TotalErrors)
	}

	if snapshot.LatencyCount != 1 {
		t.Errorf("expected latency count 1, got %d", snapshot.LatencyCount)
	}
}

func TestCollector_ConcurrentAccess(t *testing.T) {
	c := NewCollector()

	const numGoroutines = 10
	const numOperations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				c.RecordRequest("openai", "gpt-4", 200, time.Millisecond, 10)
				c.RecordError("openai", "gpt-4", "timeout")
				c.RecordRateLimitHit("acc-1", domain.LimitTypeRPM)
				c.GetSnapshot()
			}
		}(i)
	}

	wg.Wait()

	snapshot := c.GetSnapshot()

	if snapshot.TotalRequests() != int64(numGoroutines*numOperations) {
		t.Errorf("expected total requests %d, got %d", numGoroutines*numOperations, snapshot.TotalRequests())
	}
}

func TestSnapshot_LatencyPercentiles(t *testing.T) {
	c := NewCollector()

	latencies := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
		60 * time.Millisecond,
		70 * time.Millisecond,
		80 * time.Millisecond,
		90 * time.Millisecond,
		100 * time.Millisecond,
	}

	for _, l := range latencies {
		c.RecordRequest("openai", "gpt-4", 200, l, 0)
	}

	snapshot := c.GetSnapshot()
	p50, p95, p99 := snapshot.LatencyPercentiles()

	if p50 < 40*time.Millisecond || p50 > 60*time.Millisecond {
		t.Errorf("p50 latency %v outside expected range [40ms, 60ms]", p50)
	}

	if p95 < 80*time.Millisecond {
		t.Errorf("p95 latency %v less than expected 80ms", p95)
	}

	if p99 < 90*time.Millisecond {
		t.Errorf("p99 latency %v less than expected 90ms", p99)
	}
}

func TestSnapshot_LatencyPercentiles_Empty(t *testing.T) {
	c := NewCollector()
	snapshot := c.GetSnapshot()

	p50, p95, p99 := snapshot.LatencyPercentiles()

	if p50 != 0 || p95 != 0 || p99 != 0 {
		t.Errorf("expected zero latencies for empty collector, got p50=%v, p95=%v, p99=%v", p50, p95, p99)
	}
}

func TestSnapshot_RequestsByProvider(t *testing.T) {
	c := NewCollector()

	c.RecordRequest("openai", "gpt-4", 200, time.Millisecond, 0)
	c.RecordRequest("openai", "gpt-3.5-turbo", 200, time.Millisecond, 0)
	c.RecordRequest("groq", "llama-2", 200, time.Millisecond, 0)

	snapshot := c.GetSnapshot()
	byProvider := snapshot.RequestsByProvider()

	if byProvider["openai"] != 2 {
		t.Errorf("expected openai requests 2, got %d", byProvider["openai"])
	}

	if byProvider["groq"] != 1 {
		t.Errorf("expected groq requests 1, got %d", byProvider["groq"])
	}
}

func TestSnapshot_TokensByProvider(t *testing.T) {
	c := NewCollector()

	c.RecordRequest("openai", "gpt-4", 200, time.Millisecond, 100)
	c.RecordRequest("openai", "gpt-3.5-turbo", 200, time.Millisecond, 50)
	c.RecordRequest("groq", "llama-2", 200, time.Millisecond, 75)

	snapshot := c.GetSnapshot()
	byProvider := snapshot.TokensByProvider()

	if byProvider["openai"] != 150 {
		t.Errorf("expected openai tokens 150, got %d", byProvider["openai"])
	}

	if byProvider["groq"] != 75 {
		t.Errorf("expected groq tokens 75, got %d", byProvider["groq"])
	}
}

func TestReporter_Prometheus(t *testing.T) {
	c := NewCollector()

	c.RecordRequest("openai", "gpt-4", 200, 100*time.Millisecond, 100)
	c.RecordRequest("openai", "gpt-4", 200, 200*time.Millisecond, 150)
	c.RecordError("openai", "gpt-4", "timeout")
	c.RecordRateLimitHit("acc-1", domain.LimitTypeRPM)

	r := NewReporter(c)
	output := r.Prometheus()

	if !strings.Contains(output, "aiproxy_requests_total") {
		t.Error("expected aiproxy_requests_total in output")
	}

	if !strings.Contains(output, "aiproxy_request_duration_seconds") {
		t.Error("expected aiproxy_request_duration_seconds in output")
	}

	if !strings.Contains(output, "aiproxy_tokens_total") {
		t.Error("expected aiproxy_tokens_total in output")
	}

	if !strings.Contains(output, "aiproxy_errors_total") {
		t.Error("expected aiproxy_errors_total in output")
	}

	if !strings.Contains(output, "aiproxy_ratelimit_hits_total") {
		t.Error("expected aiproxy_ratelimit_hits_total in output")
	}

	if !strings.Contains(output, `provider="openai"`) {
		t.Error("expected provider label in output")
	}

	if !strings.Contains(output, `model="gpt-4"`) {
		t.Error("expected model label in output")
	}

	if !strings.Contains(output, `error_type="timeout"`) {
		t.Error("expected error_type label in output")
	}

	if !strings.Contains(output, `limit_type="rpm"`) {
		t.Error("expected limit_type label in output")
	}
}

func TestReporter_Prometheus_Quantiles(t *testing.T) {
	c := NewCollector()

	for i := 1; i <= 100; i++ {
		c.RecordRequest("openai", "gpt-4", 200, time.Duration(i)*time.Millisecond, 0)
	}

	r := NewReporter(c)
	output := r.Prometheus()

	if !strings.Contains(output, `quantile="0.5"`) {
		t.Error("expected quantile 0.5 in output")
	}

	if !strings.Contains(output, `quantile="0.95"`) {
		t.Error("expected quantile 0.95 in output")
	}

	if !strings.Contains(output, `quantile="0.99"`) {
		t.Error("expected quantile 0.99 in output")
	}
}

func TestReporter_JSON(t *testing.T) {
	c := NewCollector()

	c.RecordRequest("openai", "gpt-4", 200, 100*time.Millisecond, 100)
	c.RecordRequest("groq", "llama-2", 200, 50*time.Millisecond, 50)
	c.RecordError("openai", "gpt-4", "timeout")
	c.RecordRateLimitHit("acc-1", domain.LimitTypeRPM)

	r := NewReporter(c)
	output := r.JSON()

	if !strings.Contains(output, `"total_requests"`) {
		t.Error("expected total_requests in JSON output")
	}

	if !strings.Contains(output, `"total_tokens"`) {
		t.Error("expected total_tokens in JSON output")
	}

	if !strings.Contains(output, `"total_errors"`) {
		t.Error("expected total_errors in JSON output")
	}

	if !strings.Contains(output, `"latency_percentiles"`) {
		t.Error("expected latency_percentiles in JSON output")
	}

	if !strings.Contains(output, `"requests_by_provider"`) {
		t.Error("expected requests_by_provider in JSON output")
	}

	if !strings.Contains(output, `"tokens_by_provider"`) {
		t.Error("expected tokens_by_provider in JSON output")
	}

	if !strings.Contains(output, `"errors"`) {
		t.Error("expected errors in JSON output")
	}

	if !strings.Contains(output, `"rate_limits"`) {
		t.Error("expected rate_limits in JSON output")
	}
}

func TestReporter_JSON_ValidJSON(t *testing.T) {
	c := NewCollector()
	c.RecordRequest("openai", "gpt-4", 200, 100*time.Millisecond, 100)

	r := NewReporter(c)
	output := r.JSON()

	if !strings.HasPrefix(output, "{") {
		t.Error("expected JSON to start with {")
	}

	if !strings.HasSuffix(strings.TrimSpace(output), "}") {
		t.Error("expected JSON to end with }")
	}
}

func TestHandler_ServePrometheus(t *testing.T) {
	c := NewCollector()
	c.RecordRequest("openai", "gpt-4", 200, 100*time.Millisecond, 100)

	r := NewReporter(c)
	h := NewHandler(r)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	h.ServePrometheus(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("expected Content-Type text/plain; charset=utf-8, got %s", ct)
	}

	if !strings.Contains(rec.Body.String(), "aiproxy_requests_total") {
		t.Error("expected metrics in response body")
	}
}

func TestHandler_ServeJSON(t *testing.T) {
	c := NewCollector()
	c.RecordRequest("openai", "gpt-4", 200, 100*time.Millisecond, 100)

	r := NewReporter(c)
	h := NewHandler(r)

	req := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
	rec := httptest.NewRecorder()

	h.ServeJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	if !strings.Contains(rec.Body.String(), "total_requests") {
		t.Error("expected total_requests in response body")
	}
}

func TestHandler_Endpoints(t *testing.T) {
	c := NewCollector()

	c.RecordRequest("openai", "gpt-4", 200, 100*time.Millisecond, 100)
	c.RecordRequest("openai", "gpt-4", 500, 50*time.Millisecond, 0)
	c.RecordRequest("groq", "llama-2", 200, 75*time.Millisecond, 50)
	c.RecordError("openai", "gpt-4", "timeout")
	c.RecordRateLimitHit("acc-1", domain.LimitTypeRPM)

	r := NewReporter(c)
	h := NewHandler(r)

	t.Run("prometheus endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		rec := httptest.NewRecorder()
		h.ServePrometheus(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}

		body := rec.Body.String()
		if !strings.Contains(body, "aiproxy_requests_total{provider=\"openai\",model=\"gpt-4\"} 2") {
			t.Error("expected correct request count for openai/gpt-4")
		}
		if !strings.Contains(body, "aiproxy_tokens_total{provider=\"openai\",model=\"gpt-4\"} 100") {
			t.Error("expected correct token count for openai/gpt-4")
		}
	})

	t.Run("json endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
		rec := httptest.NewRecorder()
		h.ServeJSON(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}

		body := rec.Body.String()
		if !strings.Contains(body, `"openai": 2`) && !strings.Contains(body, `"openai":2`) {
			t.Error("expected correct request count for openai in JSON")
		}
	})
}

func TestSplitKey(t *testing.T) {
	tests := []struct {
		key      string
		expected int
	}{
		{"openai:gpt-4", 2},
		{"openai:gpt-4:timeout", 3},
		{"acc-1:rpm", 2},
		{"single", 1},
		{"", 1},
	}

	for _, tt := range tests {
		parts := splitKey(tt.key, tt.expected)
		if len(parts) == 0 && tt.key != "" {
			t.Errorf("splitKey(%q) returned empty parts", tt.key)
		}
	}
}

func (s *Snapshot) TotalRequests() int64 {
	var total int64
	for _, m := range s.Requests {
		total += m.Count
	}
	return total
}
