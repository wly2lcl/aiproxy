package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/wangluyao/aiproxy/internal/domain"
)

func Validate(cfg *Config) error {
	if err := validateServerConfig(&cfg.Server); err != nil {
		return err
	}
	if err := validateDatabaseConfig(&cfg.Database); err != nil {
		return err
	}
	if err := validateLoggingConfig(&cfg.Logging); err != nil {
		return err
	}
	if err := validateAuthConfig(&cfg.Auth); err != nil {
		return err
	}
	if err := validateProviders(&cfg.Providers); err != nil {
		return err
	}
	if err := validateFallbackConfig(&cfg.Fallback); err != nil {
		return err
	}
	if err := validateAdminConfig(&cfg.Admin); err != nil {
		return err
	}
	if err := validateMetricsConfig(&cfg.Metrics); err != nil {
		return err
	}
	if err := validateTokenTrackingConfig(&cfg.TokenTracking); err != nil {
		return err
	}
	if err := validateRateLimitsConfig(&cfg.RateLimits); err != nil {
		return err
	}
	if err := validateRequestIDConfig(&cfg.RequestID); err != nil {
		return err
	}
	if err := validateCORSConfig(&cfg.CORS); err != nil {
		return err
	}
	if err := validateSecurityHeadersConfig(&cfg.SecurityHeaders); err != nil {
		return err
	}
	return nil
}

func validateServerConfig(cfg *ServerConfig) error {
	if cfg.Port < 0 || cfg.Port > 65535 {
		return newConfigError("server.port", "must be between 0 and 65535", cfg.Port)
	}
	if cfg.ReadTimeout != "" {
		if _, err := time.ParseDuration(cfg.ReadTimeout); err != nil {
			return newConfigError("server.read_timeout", "must be a valid duration", cfg.ReadTimeout)
		}
	}
	if cfg.WriteTimeout != "" {
		if _, err := time.ParseDuration(cfg.WriteTimeout); err != nil {
			return newConfigError("server.write_timeout", "must be a valid duration", cfg.WriteTimeout)
		}
	}
	if cfg.IdleTimeout != "" {
		if _, err := time.ParseDuration(cfg.IdleTimeout); err != nil {
			return newConfigError("server.idle_timeout", "must be a valid duration", cfg.IdleTimeout)
		}
	}
	if cfg.GracefulShutdownTimeout != "" {
		if _, err := time.ParseDuration(cfg.GracefulShutdownTimeout); err != nil {
			return newConfigError("server.graceful_shutdown_timeout", "must be a valid duration", cfg.GracefulShutdownTimeout)
		}
	}
	if cfg.MaxRequestBodySize < 0 {
		return newConfigError("server.max_request_body_size", "must be non-negative", cfg.MaxRequestBodySize)
	}
	return nil
}

func validateDatabaseConfig(cfg *DatabaseConfig) error {
	if cfg.Path == "" {
		return newConfigError("database.path", "is required", cfg.Path)
	}
	validJournalModes := map[string]bool{"DELETE": true, "TRUNCATE": true, "PERSIST": true, "MEMORY": true, "WAL": true, "OFF": true}
	if !validJournalModes[strings.ToUpper(cfg.JournalMode)] {
		return newConfigError("database.journal_mode", "must be one of DELETE, TRUNCATE, PERSIST, MEMORY, WAL, OFF", cfg.JournalMode)
	}
	validAutoVacuum := map[string]bool{"NONE": true, "FULL": true, "INCREMENTAL": true}
	if !validAutoVacuum[strings.ToUpper(cfg.AutoVacuum)] {
		return newConfigError("database.auto_vacuum", "must be one of NONE, FULL, INCREMENTAL", cfg.AutoVacuum)
	}
	return nil
}

func validateLoggingConfig(cfg *LoggingConfig) error {
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true, "fatal": true}
	if !validLevels[strings.ToLower(cfg.Level)] {
		return newConfigError("logging.level", "must be one of debug, info, warn, error, fatal", cfg.Level)
	}
	validFormats := map[string]bool{"json": true, "text": true}
	if !validFormats[strings.ToLower(cfg.Format)] {
		return newConfigError("logging.format", "must be one of json, text", cfg.Format)
	}
	validOutputs := map[string]bool{"stdout": true, "stderr": true}
	if !validOutputs[strings.ToLower(cfg.Output)] {
		return newConfigError("logging.output", "must be one of stdout, stderr", cfg.Output)
	}
	return nil
}

func validateAuthConfig(cfg *AuthConfig) error {
	if cfg.Enabled && len(cfg.APIKeys) == 0 {
		return newConfigError("auth.api_keys", "is required when auth is enabled", nil)
	}
	if cfg.HeaderName == "" {
		return newConfigError("auth.header_name", "is required", cfg.HeaderName)
	}
	return nil
}

func validateProviders(providers *[]ProviderConfig) error {
	if len(*providers) == 0 {
		return newConfigError("providers", "at least one provider is required", nil)
	}
	seenNames := make(map[string]bool)
	seenDefaults := 0
	for i, p := range *providers {
		if p.Name == "" {
			return newConfigError(fmt.Sprintf("providers[%d].name", i), "is required", p.Name)
		}
		if seenNames[p.Name] {
			return newConfigError(fmt.Sprintf("providers[%d].name", i), "must be unique", p.Name)
		}
		seenNames[p.Name] = true
		if p.APIBase == "" {
			return newConfigError(fmt.Sprintf("providers[%d].api_base", i), "is required", p.APIBase)
		}
		if _, err := url.Parse(p.APIBase); err != nil {
			return newConfigError(fmt.Sprintf("providers[%d].api_base", i), "must be a valid URL", p.APIBase)
		}
		if p.IsDefault {
			seenDefaults++
		}
		if err := validateProviderTimeouts(i, &p); err != nil {
			return err
		}
		if err := validateProviderRetry(i, &p); err != nil {
			return err
		}
		if err := validateProviderCircuitBreaker(i, &p); err != nil {
			return err
		}
		if err := validateProviderAPIKeys(i, &p); err != nil {
			return err
		}
	}
	if seenDefaults > 1 {
		return newConfigError("providers", "only one provider can be default", nil)
	}
	return nil
}

func validateProviderTimeouts(idx int, p *ProviderConfig) error {
	if p.Timeout != "" {
		if _, err := time.ParseDuration(p.Timeout); err != nil {
			return newConfigError(fmt.Sprintf("providers[%d].timeout", idx), "must be a valid duration", p.Timeout)
		}
	}
	if p.StreamTimeout != "" {
		if _, err := time.ParseDuration(p.StreamTimeout); err != nil {
			return newConfigError(fmt.Sprintf("providers[%d].stream_timeout", idx), "must be a valid duration", p.StreamTimeout)
		}
	}
	return nil
}

func validateProviderRetry(idx int, p *ProviderConfig) error {
	if p.Retry.MaxRetries < 0 {
		return newConfigError(fmt.Sprintf("providers[%d].retry.max_retries", idx), "must be non-negative", p.Retry.MaxRetries)
	}
	if p.Retry.InitialWait != "" {
		if _, err := time.ParseDuration(p.Retry.InitialWait); err != nil {
			return newConfigError(fmt.Sprintf("providers[%d].retry.initial_wait", idx), "must be a valid duration", p.Retry.InitialWait)
		}
	}
	if p.Retry.MaxWait != "" {
		if _, err := time.ParseDuration(p.Retry.MaxWait); err != nil {
			return newConfigError(fmt.Sprintf("providers[%d].retry.max_wait", idx), "must be a valid duration", p.Retry.MaxWait)
		}
	}
	if p.Retry.Multiplier < 1.0 {
		return newConfigError(fmt.Sprintf("providers[%d].retry.multiplier", idx), "must be at least 1.0", p.Retry.Multiplier)
	}
	return nil
}

func validateProviderCircuitBreaker(idx int, p *ProviderConfig) error {
	if p.CircuitBreaker.Threshold < 0 {
		return newConfigError(fmt.Sprintf("providers[%d].circuit_breaker.threshold", idx), "must be non-negative", p.CircuitBreaker.Threshold)
	}
	if p.CircuitBreaker.Timeout != "" {
		if _, err := time.ParseDuration(p.CircuitBreaker.Timeout); err != nil {
			return newConfigError(fmt.Sprintf("providers[%d].circuit_breaker.timeout", idx), "must be a valid duration", p.CircuitBreaker.Timeout)
		}
	}
	if p.CircuitBreaker.HalfOpenRequests < 0 {
		return newConfigError(fmt.Sprintf("providers[%d].circuit_breaker.half_open_requests", idx), "must be non-negative", p.CircuitBreaker.HalfOpenRequests)
	}
	return nil
}

func validateProviderAPIKeys(idx int, p *ProviderConfig) error {
	if len(p.APIKeys) == 0 {
		return newConfigError(fmt.Sprintf("providers[%d].api_keys", idx), "at least one API key is required", nil)
	}
	seenKeys := make(map[string]bool)
	for j, key := range p.APIKeys {
		if key.Key == "" {
			return newConfigError(fmt.Sprintf("providers[%d].api_keys[%d].key", idx, j), "is required", key.Key)
		}
		if seenKeys[key.Key] {
			return newConfigError(fmt.Sprintf("providers[%d].api_keys[%d].key", idx, j), "must be unique", key.Key)
		}
		seenKeys[key.Key] = true
		if key.Weight < 0 {
			return newConfigError(fmt.Sprintf("providers[%d].api_keys[%d].weight", idx, j), "must be non-negative", key.Weight)
		}
		if key.Priority < 0 {
			return newConfigError(fmt.Sprintf("providers[%d].api_keys[%d].priority", idx, j), "must be non-negative", key.Priority)
		}
		if err := validateAccountLimits(idx, j, key.Limits); err != nil {
			return err
		}
	}
	return nil
}

func validateAccountLimits(providerIdx, keyIdx int, limits *domain.AccountLimits) error {
	if limits == nil {
		return nil
	}
	if limits.RPM != nil && *limits.RPM < 0 {
		return newConfigError(fmt.Sprintf("providers[%d].api_keys[%d].limits.rpm", providerIdx, keyIdx), "must be non-negative", *limits.RPM)
	}
	if limits.Daily != nil && *limits.Daily < 0 {
		return newConfigError(fmt.Sprintf("providers[%d].api_keys[%d].limits.daily", providerIdx, keyIdx), "must be non-negative", *limits.Daily)
	}
	if limits.Window5h != nil && *limits.Window5h < 0 {
		return newConfigError(fmt.Sprintf("providers[%d].api_keys[%d].limits.window_5h", providerIdx, keyIdx), "must be non-negative", *limits.Window5h)
	}
	if limits.Monthly != nil && *limits.Monthly < 0 {
		return newConfigError(fmt.Sprintf("providers[%d].api_keys[%d].limits.monthly", providerIdx, keyIdx), "must be non-negative", *limits.Monthly)
	}
	if limits.TokenDaily != nil && *limits.TokenDaily < 0 {
		return newConfigError(fmt.Sprintf("providers[%d].api_keys[%d].limits.token_daily", providerIdx, keyIdx), "must be non-negative", *limits.TokenDaily)
	}
	if limits.TokenMonthly != nil && *limits.TokenMonthly < 0 {
		return newConfigError(fmt.Sprintf("providers[%d].api_keys[%d].limits.token_monthly", providerIdx, keyIdx), "must be non-negative", *limits.TokenMonthly)
	}
	return nil
}

func validateFallbackConfig(cfg *FallbackConfig) error {
	if cfg.Enabled && len(cfg.Providers) == 0 {
		return newConfigError("fallback.providers", "at least one provider is required when fallback is enabled", nil)
	}
	validStrategies := map[string]bool{"sequential": true, "parallel": true, "round_robin": true}
	if cfg.Enabled && !validStrategies[cfg.Strategy] {
		return newConfigError("fallback.strategy", "must be one of sequential, parallel, round_robin", cfg.Strategy)
	}
	return nil
}

func validateAdminConfig(cfg *AdminConfig) error {
	if cfg.Enabled && len(cfg.APIKeys) == 0 {
		return newConfigError("admin.api_keys", "is required when admin is enabled (for security)", nil)
	}
	if cfg.RateLimit < 0 {
		return newConfigError("admin.rate_limit", "must be non-negative", cfg.RateLimit)
	}
	return nil
}

func validateMetricsConfig(cfg *MetricsConfig) error {
	if cfg.Prometheus.Enabled && cfg.Prometheus.Path == "" {
		return newConfigError("metrics.prometheus.path", "is required when prometheus is enabled", cfg.Prometheus.Path)
	}
	return nil
}

func validateTokenTrackingConfig(cfg *TokenTrackingConfig) error {
	if cfg.Enabled {
		validModes := map[string]bool{"hybrid": true, "streaming": true, "response": true}
		if !validModes[cfg.StreamingMode] {
			return newConfigError("token_tracking.streaming_mode", "must be one of hybrid, streaming, response", cfg.StreamingMode)
		}
		if cfg.EstimationCharsPerToken <= 0 {
			return newConfigError("token_tracking.estimation_chars_per_token", "must be positive", cfg.EstimationCharsPerToken)
		}
		if cfg.ReconciliationInterval != "" {
			if _, err := time.ParseDuration(cfg.ReconciliationInterval); err != nil {
				return newConfigError("token_tracking.reconciliation_interval", "must be a valid duration", cfg.ReconciliationInterval)
			}
		}
	}
	return nil
}

func validateRateLimitsConfig(cfg *RateLimitsConfig) error {
	if cfg.CleanupInterval != "" {
		if _, err := time.ParseDuration(cfg.CleanupInterval); err != nil {
			return newConfigError("rate_limits.cleanup_interval", "must be a valid duration", cfg.CleanupInterval)
		}
	}
	if cfg.Window5hDuration != "" {
		if _, err := time.ParseDuration(cfg.Window5hDuration); err != nil {
			return newConfigError("rate_limits.window_5h_duration", "must be a valid duration", cfg.Window5hDuration)
		}
	}
	return nil
}

func validateRequestIDConfig(cfg *RequestIDConfig) error {
	if cfg.HeaderName == "" {
		return newConfigError("request_id.header_name", "is required", cfg.HeaderName)
	}
	return nil
}

func validateCORSConfig(cfg *CORSConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if len(cfg.AllowedOrigins) == 0 {
		return newConfigError("cors.allowed_origins", "is required when CORS is enabled", nil)
	}
	if cfg.MaxAge < 0 {
		return newConfigError("cors.max_age", "must be non-negative", cfg.MaxAge)
	}
	if cfg.AllowCredentials {
		for _, o := range cfg.AllowedOrigins {
			if o == "*" {
				return newConfigError("cors.allowed_origins", "cannot contain '*' when allow_credentials is true (browsers reject this for security)", nil)
			}
		}
	}
	return nil
}

func validateSecurityHeadersConfig(cfg *SecurityHeadersConfig) error {
	if !cfg.Enabled {
		return nil
	}
	return nil
}

func newConfigError(field, message string, value interface{}) error {
	return &domain.DomainError{
		Code:    domain.ErrCodeInvalidConfig,
		Message: fmt.Sprintf("config validation failed: %s %s", field, message),
		Details: map[string]interface{}{
			"field":   field,
			"message": message,
			"value":   value,
		},
	}
}
