package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/wangluyao/aiproxy/internal/domain"
)

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	return LoadFromBytes(data)
}

func LoadFromBytes(data []byte) (*Config, error) {
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	ApplyDefaults(&cfg)
	ApplyEnvironmentOverrides(&cfg)
	return &cfg, nil
}

func ApplyDefaults(cfg *Config) {
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.ReadTimeout == "" {
		cfg.Server.ReadTimeout = "30s"
	}
	if cfg.Server.WriteTimeout == "" {
		cfg.Server.WriteTimeout = "120s"
	}
	if cfg.Server.IdleTimeout == "" {
		cfg.Server.IdleTimeout = "120s"
	}
	if cfg.Server.GracefulShutdownTimeout == "" {
		cfg.Server.GracefulShutdownTimeout = "30s"
	}
	if cfg.Server.MaxRequestBodySize == 0 {
		cfg.Server.MaxRequestBodySize = 10 * 1024 * 1024
	}

	if cfg.Database.Path == "" {
		cfg.Database.Path = "data/aiproxy.db"
	}
	if cfg.Database.BusyTimeout == 0 {
		cfg.Database.BusyTimeout = 5000
	}
	if cfg.Database.JournalMode == "" {
		cfg.Database.JournalMode = "WAL"
	}
	if cfg.Database.CacheSize == 0 {
		cfg.Database.CacheSize = -64000
	}
	if cfg.Database.AutoVacuum == "" {
		cfg.Database.AutoVacuum = "INCREMENTAL"
	}

	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}
	if cfg.Logging.Output == "" {
		cfg.Logging.Output = "stdout"
	}

	if cfg.Auth.HeaderName == "" {
		cfg.Auth.HeaderName = "Authorization"
	}
	if cfg.Auth.KeyPrefix == "" {
		cfg.Auth.KeyPrefix = "Bearer "
	}

	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		if p.Timeout == "" {
			p.Timeout = "30s"
		}
		if p.StreamTimeout == "" {
			p.StreamTimeout = "120s"
		}
		if p.Retry.MaxRetries == 0 {
			p.Retry.MaxRetries = 3
		}
		if p.Retry.InitialWait == "" {
			p.Retry.InitialWait = "1s"
		}
		if p.Retry.MaxWait == "" {
			p.Retry.MaxWait = "30s"
		}
		if p.Retry.Multiplier == 0 {
			p.Retry.Multiplier = 2.0
		}
		if p.CircuitBreaker.Threshold == 0 {
			p.CircuitBreaker.Threshold = 5
		}
		if p.CircuitBreaker.Timeout == "" {
			p.CircuitBreaker.Timeout = "60s"
		}
		if p.CircuitBreaker.HalfOpenRequests == 0 {
			p.CircuitBreaker.HalfOpenRequests = 1
		}
		for j := range p.APIKeys {
			key := &p.APIKeys[j]
			if key.Weight == 0 {
				key.Weight = 1
			}
			if !key.IsEnabled && key.Key != "" {
				key.IsEnabled = true
			}
		}
	}

	if cfg.Fallback.Strategy == "" {
		cfg.Fallback.Strategy = "sequential"
	}

	if cfg.Admin.Listen == "" {
		cfg.Admin.Listen = "127.0.0.1:8081"
	}
	if cfg.Admin.RateLimit == 0 {
		cfg.Admin.RateLimit = 100
	}

	if cfg.Metrics.Namespace == "" {
		cfg.Metrics.Namespace = "aiproxy"
	}
	if cfg.Metrics.Prometheus.Path == "" {
		cfg.Metrics.Prometheus.Path = "/metrics"
	}

	if cfg.TokenTracking.StreamingMode == "" {
		cfg.TokenTracking.StreamingMode = "hybrid"
	}
	if cfg.TokenTracking.EstimationCharsPerToken == 0 {
		cfg.TokenTracking.EstimationCharsPerToken = 4
	}
	if cfg.TokenTracking.ReconciliationInterval == "" {
		cfg.TokenTracking.ReconciliationInterval = "5m"
	}

	if cfg.RateLimits.CleanupInterval == "" {
		cfg.RateLimits.CleanupInterval = "1h"
	}
	if cfg.RateLimits.Window5hDuration == "" {
		cfg.RateLimits.Window5hDuration = "5h"
	}

	if cfg.RequestID.HeaderName == "" {
		cfg.RequestID.HeaderName = "X-Request-ID"
	}
}

func ApplyEnvironmentOverrides(cfg *Config) {
	applyEnvString("AIPROXY_SERVER_HOST", &cfg.Server.Host)
	applyEnvInt("AIPROXY_SERVER_PORT", &cfg.Server.Port)
	applyEnvString("AIPROXY_SERVER_READ_TIMEOUT", &cfg.Server.ReadTimeout)
	applyEnvString("AIPROXY_SERVER_WRITE_TIMEOUT", &cfg.Server.WriteTimeout)
	applyEnvString("AIPROXY_SERVER_IDLE_TIMEOUT", &cfg.Server.IdleTimeout)
	applyEnvString("AIPROXY_SERVER_GRACEFUL_SHUTDOWN_TIMEOUT", &cfg.Server.GracefulShutdownTimeout)
	applyEnvInt64("AIPROXY_SERVER_MAX_REQUEST_BODY_SIZE", &cfg.Server.MaxRequestBodySize)

	applyEnvString("AIPROXY_DATABASE_PATH", &cfg.Database.Path)
	applyEnvInt("AIPROXY_DATABASE_BUSY_TIMEOUT", &cfg.Database.BusyTimeout)
	applyEnvString("AIPROXY_DATABASE_JOURNAL_MODE", &cfg.Database.JournalMode)
	applyEnvInt("AIPROXY_DATABASE_CACHE_SIZE", &cfg.Database.CacheSize)
	applyEnvString("AIPROXY_DATABASE_AUTO_VACUUM", &cfg.Database.AutoVacuum)

	applyEnvString("AIPROXY_LOGGING_LEVEL", &cfg.Logging.Level)
	applyEnvString("AIPROXY_LOGGING_FORMAT", &cfg.Logging.Format)
	applyEnvString("AIPROXY_LOGGING_OUTPUT", &cfg.Logging.Output)
	applyEnvBool("AIPROXY_LOGGING_INCLUDE_REQUEST_BODY", &cfg.Logging.IncludeRequestBody)
	applyEnvBool("AIPROXY_LOGGING_INCLUDE_RESPONSE_BODY", &cfg.Logging.IncludeResponseBody)

	applyEnvBool("AIPROXY_AUTH_ENABLED", &cfg.Auth.Enabled)
	applyEnvStringSlice("AIPROXY_AUTH_API_KEYS", &cfg.Auth.APIKeys)
	applyEnvString("AIPROXY_AUTH_HEADER_NAME", &cfg.Auth.HeaderName)
	applyEnvString("AIPROXY_AUTH_KEY_PREFIX", &cfg.Auth.KeyPrefix)

	applyEnvBool("AIPROXY_FALLBACK_ENABLED", &cfg.Fallback.Enabled)
	applyEnvString("AIPROXY_FALLBACK_STRATEGY", &cfg.Fallback.Strategy)
	applyEnvStringSlice("AIPROXY_FALLBACK_PROVIDERS", &cfg.Fallback.Providers)

	applyEnvBool("AIPROXY_ADMIN_ENABLED", &cfg.Admin.Enabled)
	applyEnvString("AIPROXY_ADMIN_LISTEN", &cfg.Admin.Listen)
	applyEnvStringSlice("AIPROXY_ADMIN_API_KEYS", &cfg.Admin.APIKeys)
	applyEnvInt("AIPROXY_ADMIN_RATE_LIMIT", &cfg.Admin.RateLimit)

	applyEnvBool("AIPROXY_METRICS_ENABLED", &cfg.Metrics.Enabled)
	applyEnvString("AIPROXY_METRICS_NAMESPACE", &cfg.Metrics.Namespace)
	applyEnvBool("AIPROXY_METRICS_PROMETHEUS_ENABLED", &cfg.Metrics.Prometheus.Enabled)
	applyEnvString("AIPROXY_METRICS_PROMETHEUS_PATH", &cfg.Metrics.Prometheus.Path)
	applyEnvBool("AIPROXY_METRICS_JSON_ENABLED", &cfg.Metrics.JSON.Enabled)

	applyEnvBool("AIPROXY_TOKEN_TRACKING_ENABLED", &cfg.TokenTracking.Enabled)
	applyEnvString("AIPROXY_TOKEN_TRACKING_STREAMING_MODE", &cfg.TokenTracking.StreamingMode)
	applyEnvInt("AIPROXY_TOKEN_TRACKING_ESTIMATION_CHARS_PER_TOKEN", &cfg.TokenTracking.EstimationCharsPerToken)
	applyEnvString("AIPROXY_TOKEN_TRACKING_RECONCILIATION_INTERVAL", &cfg.TokenTracking.ReconciliationInterval)

	applyEnvString("AIPROXY_RATE_LIMITS_CLEANUP_INTERVAL", &cfg.RateLimits.CleanupInterval)
	applyEnvString("AIPROXY_RATE_LIMITS_WINDOW_5H_DURATION", &cfg.RateLimits.Window5hDuration)

	applyEnvString("AIPROXY_REQUEST_ID_HEADER_NAME", &cfg.RequestID.HeaderName)
	applyEnvBool("AIPROXY_REQUEST_ID_GENERATE_IF_MISSING", &cfg.RequestID.GenerateIfMissing)

	applyEnvProviderConfigs(&cfg.Providers)
}

func applyEnvProviderConfigs(providers *[]ProviderConfig) {
	envKeys := os.Getenv("AIPROXY_PROVIDERS_API_KEYS")
	if envKeys == "" {
		return
	}

	var envProviderConfigs []struct {
		Name    string             `json:"name"`
		APIKeys []AccountKeyConfig `json:"api_keys"`
	}
	if err := json.Unmarshal([]byte(envKeys), &envProviderConfigs); err == nil {
		for i, envProvider := range envProviderConfigs {
			for j := range *providers {
				if (*providers)[j].Name == envProvider.Name {
					(*providers)[j].APIKeys = envProvider.APIKeys
					break
				}
			}
			if i >= len(*providers) || (*providers)[len(*providers)-1].Name != envProvider.Name {
				newProvider := ProviderConfig{
					Name:    envProvider.Name,
					APIKeys: envProvider.APIKeys,
				}
				*providers = append(*providers, newProvider)
			}
		}
	}
}

func applyEnvString(key string, target *string) {
	if val := os.Getenv(key); val != "" {
		*target = val
	}
}

func applyEnvInt(key string, target *int) {
	if val := os.Getenv(key); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			*target = intVal
		}
	}
}

func applyEnvInt64(key string, target *int64) {
	if val := os.Getenv(key); val != "" {
		if intVal, err := strconv.ParseInt(val, 10, 64); err == nil {
			*target = intVal
		}
	}
}

func applyEnvBool(key string, target *bool) {
	if val := os.Getenv(key); val != "" {
		if boolVal, err := strconv.ParseBool(val); err == nil {
			*target = boolVal
		}
	}
}

func applyEnvStringSlice(key string, target *[]string) {
	if val := os.Getenv(key); val != "" {
		if val == "[]" {
			*target = []string{}
			return
		}
		var slice []string
		if err := json.Unmarshal([]byte(val), &slice); err == nil {
			*target = slice
		} else {
			*target = strings.Split(val, ",")
		}
	}
}

func applyEnvAccountLimits(key string, target **domain.AccountLimits) {
	if val := os.Getenv(key); val != "" {
		var limits domain.AccountLimits
		if err := json.Unmarshal([]byte(val), &limits); err == nil {
			*target = &limits
		}
	}
}
