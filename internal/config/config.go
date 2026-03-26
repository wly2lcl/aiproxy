package config

import (
	"github.com/wangluyao/aiproxy/internal/domain"
)

type ServerConfig struct {
	Host                    string `json:"host"`
	Port                    int    `json:"port"`
	ReadTimeout             string `json:"read_timeout"`
	WriteTimeout            string `json:"write_timeout"`
	IdleTimeout             string `json:"idle_timeout"`
	GracefulShutdownTimeout string `json:"graceful_shutdown_timeout"`
	MaxRequestBodySize      int64  `json:"max_request_body_size"`
}

type DatabaseConfig struct {
	Path        string `json:"path"`
	BusyTimeout int    `json:"busy_timeout"`
	JournalMode string `json:"journal_mode"`
	CacheSize   int    `json:"cache_size"`
	AutoVacuum  string `json:"auto_vacuum"`
}

type LoggingConfig struct {
	Level               string `json:"level"`
	Format              string `json:"format"`
	Output              string `json:"output"`
	IncludeRequestBody  bool   `json:"include_request_body"`
	IncludeResponseBody bool   `json:"include_response_body"`
}

type AuthConfig struct {
	Enabled    bool     `json:"enabled"`
	APIKeys    []string `json:"api_keys"`
	HeaderName string   `json:"header_name"`
	KeyPrefix  string   `json:"key_prefix"`
}

type AccountKeyConfig struct {
	Key       string                `json:"key"`
	Name      string                `json:"name"`
	Weight    int                   `json:"weight"`
	Priority  int                   `json:"priority"`
	IsEnabled bool                  `json:"is_enabled"`
	Limits    *domain.AccountLimits `json:"limits"`
}

type RetryConfig struct {
	MaxRetries  int     `json:"max_retries"`
	InitialWait string  `json:"initial_wait"`
	MaxWait     string  `json:"max_wait"`
	Multiplier  float64 `json:"multiplier"`
}

type CircuitBreakerConfig struct {
	Threshold        int    `json:"threshold"`
	Timeout          string `json:"timeout"`
	HalfOpenRequests int    `json:"half_open_requests"`
}

type ProviderConfig struct {
	Name           string               `json:"name"`
	APIBase        string               `json:"api_base"`
	Models         []string             `json:"models"`
	IsDefault      bool                 `json:"is_default"`
	IsEnabled      bool                 `json:"is_enabled"`
	Headers        map[string]string    `json:"headers"`
	Timeout        string               `json:"timeout"`
	StreamTimeout  string               `json:"stream_timeout"`
	APIKeys        []AccountKeyConfig   `json:"api_keys"`
	Retry          RetryConfig          `json:"retry"`
	CircuitBreaker CircuitBreakerConfig `json:"circuit_breaker"`
}

type ModelMappingConfig map[string]string

type FallbackConfig struct {
	Enabled   bool     `json:"enabled"`
	Strategy  string   `json:"strategy"`
	Providers []string `json:"providers"`
}

type AdminConfig struct {
	Enabled   bool     `json:"enabled"`
	Listen    string   `json:"listen"`
	APIKeys   []string `json:"api_keys"`
	RateLimit int      `json:"rate_limit"`
}

type PrometheusConfig struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
}

type JSONMetricsConfig struct {
	Enabled bool `json:"enabled"`
}

type MetricsConfig struct {
	Enabled    bool              `json:"enabled"`
	Prometheus PrometheusConfig  `json:"prometheus"`
	JSON       JSONMetricsConfig `json:"json"`
	Namespace  string            `json:"namespace"`
}

type TokenTrackingConfig struct {
	Enabled                 bool   `json:"enabled"`
	StreamingMode           string `json:"streaming_mode"`
	EstimationCharsPerToken int    `json:"estimation_chars_per_token"`
	ReconciliationInterval  string `json:"reconciliation_interval"`
}

type RateLimitsConfig struct {
	CleanupInterval  string `json:"cleanup_interval"`
	Window5hDuration string `json:"window_5h_duration"`
}

type RequestIDConfig struct {
	HeaderName        string `json:"header_name"`
	GenerateIfMissing bool   `json:"generate_if_missing"`
}

type Config struct {
	Server        ServerConfig        `json:"server"`
	Database      DatabaseConfig      `json:"database"`
	Logging       LoggingConfig       `json:"logging"`
	Auth          AuthConfig          `json:"auth"`
	Providers     []ProviderConfig    `json:"providers"`
	ModelMapping  ModelMappingConfig  `json:"model_mapping"`
	Fallback      FallbackConfig      `json:"fallback"`
	Admin         AdminConfig         `json:"admin"`
	Metrics       MetricsConfig       `json:"metrics"`
	TokenTracking TokenTrackingConfig `json:"token_tracking"`
	RateLimits    RateLimitsConfig    `json:"rate_limits"`
	RequestID     RequestIDConfig     `json:"request_id"`
}
