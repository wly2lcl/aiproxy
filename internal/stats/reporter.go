package stats

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Reporter struct {
	collector *Collector
	namespace string
}

func NewReporter(collector *Collector) *Reporter {
	return &Reporter{
		collector: collector,
		namespace: "aiproxy",
	}
}

func NewReporterWithNamespace(collector *Collector, namespace string) *Reporter {
	if namespace == "" {
		namespace = "aiproxy"
	}
	return &Reporter{
		collector: collector,
		namespace: namespace,
	}
}

func (r *Reporter) Prometheus() string {
	snapshot := r.collector.GetSnapshot()
	ns := r.namespace

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# HELP %s_requests_total Total number of requests\n", ns))
	sb.WriteString(fmt.Sprintf("# TYPE %s_requests_total counter\n", ns))
	for key, m := range snapshot.Requests {
		parts := splitKey(key, 2)
		if len(parts) == 2 {
			sb.WriteString(fmt.Sprintf("%s_requests_total{provider=%q,model=%q} %d\n",
				ns, parts[0], parts[1], m.Count))
		}
	}

	sb.WriteString(fmt.Sprintf("\n# HELP %s_request_duration_seconds Request duration in seconds\n", ns))
	sb.WriteString(fmt.Sprintf("# TYPE %s_request_duration_seconds summary\n", ns))
	p50, p95, p99 := snapshot.LatencyPercentiles()
	sb.WriteString(fmt.Sprintf("%s_request_duration_seconds{quantile=\"0.5\"} %f\n", ns, p50.Seconds()))
	sb.WriteString(fmt.Sprintf("%s_request_duration_seconds{quantile=\"0.95\"} %f\n", ns, p95.Seconds()))
	sb.WriteString(fmt.Sprintf("%s_request_duration_seconds{quantile=\"0.99\"} %f\n", ns, p99.Seconds()))
	if snapshot.LatencyCount > 0 {
		sb.WriteString(fmt.Sprintf("%s_request_duration_seconds_sum %f\n", ns, snapshot.TotalLatency.Seconds()))
		sb.WriteString(fmt.Sprintf("%s_request_duration_seconds_count %d\n", ns, snapshot.LatencyCount))
	}

	sb.WriteString(fmt.Sprintf("\n# HELP %s_tokens_total Total tokens used\n", ns))
	sb.WriteString(fmt.Sprintf("# TYPE %s_tokens_total counter\n", ns))
	for key, m := range snapshot.Requests {
		parts := splitKey(key, 2)
		if len(parts) == 2 {
			sb.WriteString(fmt.Sprintf("%s_tokens_total{provider=%q,model=%q} %d\n",
				ns, parts[0], parts[1], m.Tokens))
		}
	}

	sb.WriteString(fmt.Sprintf("\n# HELP %s_errors_total Total number of errors\n", ns))
	sb.WriteString(fmt.Sprintf("# TYPE %s_errors_total counter\n", ns))
	for _, e := range snapshot.Errors {
		sb.WriteString(fmt.Sprintf("%s_errors_total{provider=%q,model=%q,error_type=%q} %d\n",
			ns, e.Provider, e.Model, e.ErrorType, e.Count))
	}
	for key, m := range snapshot.Requests {
		if m.Errors > 0 {
			parts := splitKey(key, 2)
			if len(parts) == 2 {
				sb.WriteString(fmt.Sprintf("%s_errors_total{provider=%q,model=%q,error_type=%q} %d\n",
					ns, parts[0], parts[1], "http_error", m.Errors))
			}
		}
	}

	sb.WriteString(fmt.Sprintf("\n# HELP %s_ratelimit_hits_total Rate limit hits\n", ns))
	sb.WriteString(fmt.Sprintf("# TYPE %s_ratelimit_hits_total counter\n", ns))
	for _, rl := range snapshot.RateLimits {
		sb.WriteString(fmt.Sprintf("%s_ratelimit_hits_total{account_id=%q,limit_type=%q} %d\n",
			ns, rl.AccountID, rl.LimitType, rl.Count))
	}

	return sb.String()
}

func (r *Reporter) JSON() string {
	snapshot := r.collector.GetSnapshot()
	p50, p95, p99 := snapshot.LatencyPercentiles()

	data := struct {
		Timestamp          string             `json:"timestamp"`
		TotalRequests      int64              `json:"total_requests"`
		TotalErrors        int64              `json:"total_errors"`
		TotalTokens        int64              `json:"total_tokens"`
		LatencyPercentile  LatencyPercentiles `json:"latency_percentiles"`
		RequestsByProvider map[string]int64   `json:"requests_by_provider"`
		RequestsByModel    map[string]int64   `json:"requests_by_model"`
		TokensByProvider   map[string]int64   `json:"tokens_by_provider"`
		TokensByModel      map[string]int64   `json:"tokens_by_model"`
		Errors             []ErrorMetric      `json:"errors"`
		RateLimits         []RateLimitMetric  `json:"rate_limits"`
	}{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		TotalRequests: func() int64 {
			var total int64
			for _, m := range snapshot.Requests {
				total += m.Count
			}
			return total
		}(),
		TotalErrors:        snapshot.TotalErrors,
		TotalTokens:        snapshot.TotalTokens,
		LatencyPercentile:  LatencyPercentiles{P50: p50, P95: p95, P99: p99},
		RequestsByProvider: snapshot.RequestsByProvider(),
		RequestsByModel:    snapshot.RequestsByModel(),
		TokensByProvider:   snapshot.TokensByProvider(),
		TokensByModel:      snapshot.TokensByModel(),
		Errors:             snapshot.Errors,
		RateLimits:         snapshot.RateLimits,
	}

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}

type LatencyPercentiles struct {
	P50 time.Duration `json:"p50"`
	P95 time.Duration `json:"p95"`
	P99 time.Duration `json:"p99"`
}
