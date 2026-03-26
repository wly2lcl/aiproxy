package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wangluyao/aiproxy/internal/domain"
)

func TestLoad_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	configData := `{
		"server": {
			"host": "127.0.0.1",
			"port": 9090
		},
		"providers": [
			{
				"name": "test-provider",
				"api_base": "https://api.test.com/v1",
				"api_keys": [
					{"key": "test-key", "weight": 1, "is_enabled": true}
				]
			}
		]
	}`

	if err := os.WriteFile(configPath, []byte(configData), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "127.0.0.1")
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, 9090)
	}
	if len(cfg.Providers) != 1 {
		t.Errorf("len(Providers) = %d, want %d", len(cfg.Providers), 1)
	}
	if cfg.Providers[0].Name != "test-provider" {
		t.Errorf("Providers[0].Name = %q, want %q", cfg.Providers[0].Name, "test-provider")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	invalidJSON := `{invalid json`

	if err := os.WriteFile(configPath, []byte(invalidJSON), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err == nil {
		t.Errorf("Load() expected error, got nil, cfg = %+v", cfg)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.json")
	if err == nil {
		t.Errorf("Load() expected error, got nil, cfg = %+v", cfg)
	}
}

func TestLoadFromBytes(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{
			name: "valid config",
			data: `{
				"server": {"port": 8080},
				"providers": [
					{
						"name": "test",
						"api_base": "https://api.test.com",
						"api_keys": [{"key": "key1"}]
					}
				]
			}`,
			wantErr: false,
		},
		{
			name:    "invalid json",
			data:    `{invalid}`,
			wantErr: true,
		},
		{
			name:    "empty json object",
			data:    `{}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadFromBytes([]byte(tt.data))
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadFromBytes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && cfg == nil {
				t.Error("LoadFromBytes() returned nil config without error")
			}
		})
	}
}

func TestApplyDefaults(t *testing.T) {
	cfg := &Config{}
	ApplyDefaults(cfg)

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Server.Host default = %q, want %q", cfg.Server.Host, "0.0.0.0")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port default = %d, want %d", cfg.Server.Port, 8080)
	}
	if cfg.Server.ReadTimeout != "30s" {
		t.Errorf("Server.ReadTimeout default = %q, want %q", cfg.Server.ReadTimeout, "30s")
	}
	if cfg.Server.WriteTimeout != "120s" {
		t.Errorf("Server.WriteTimeout default = %q, want %q", cfg.Server.WriteTimeout, "120s")
	}
	if cfg.Server.IdleTimeout != "120s" {
		t.Errorf("Server.IdleTimeout default = %q, want %q", cfg.Server.IdleTimeout, "120s")
	}
	if cfg.Server.GracefulShutdownTimeout != "30s" {
		t.Errorf("Server.GracefulShutdownTimeout default = %q, want %q", cfg.Server.GracefulShutdownTimeout, "30s")
	}
	if cfg.Server.MaxRequestBodySize != 10*1024*1024 {
		t.Errorf("Server.MaxRequestBodySize default = %d, want %d", cfg.Server.MaxRequestBodySize, 10*1024*1024)
	}

	if cfg.Database.Path != "data/aiproxy.db" {
		t.Errorf("Database.Path default = %q, want %q", cfg.Database.Path, "data/aiproxy.db")
	}
	if cfg.Database.BusyTimeout != 5000 {
		t.Errorf("Database.BusyTimeout default = %d, want %d", cfg.Database.BusyTimeout, 5000)
	}
	if cfg.Database.JournalMode != "WAL" {
		t.Errorf("Database.JournalMode default = %q, want %q", cfg.Database.JournalMode, "WAL")
	}

	if cfg.Logging.Level != "info" {
		t.Errorf("Logging.Level default = %q, want %q", cfg.Logging.Level, "info")
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("Logging.Format default = %q, want %q", cfg.Logging.Format, "json")
	}
	if cfg.Logging.Output != "stdout" {
		t.Errorf("Logging.Output default = %q, want %q", cfg.Logging.Output, "stdout")
	}

	if cfg.Auth.HeaderName != "Authorization" {
		t.Errorf("Auth.HeaderName default = %q, want %q", cfg.Auth.HeaderName, "Authorization")
	}
	if cfg.Auth.KeyPrefix != "Bearer " {
		t.Errorf("Auth.KeyPrefix default = %q, want %q", cfg.Auth.KeyPrefix, "Bearer ")
	}

	if cfg.Admin.Listen != "127.0.0.1:8081" {
		t.Errorf("Admin.Listen default = %q, want %q", cfg.Admin.Listen, "127.0.0.1:8081")
	}
	if cfg.Admin.RateLimit != 100 {
		t.Errorf("Admin.RateLimit default = %d, want %d", cfg.Admin.RateLimit, 100)
	}

	if cfg.Metrics.Namespace != "aiproxy" {
		t.Errorf("Metrics.Namespace default = %q, want %q", cfg.Metrics.Namespace, "aiproxy")
	}
	if cfg.Metrics.Prometheus.Path != "/metrics" {
		t.Errorf("Metrics.Prometheus.Path default = %q, want %q", cfg.Metrics.Prometheus.Path, "/metrics")
	}

	if cfg.TokenTracking.StreamingMode != "hybrid" {
		t.Errorf("TokenTracking.StreamingMode default = %q, want %q", cfg.TokenTracking.StreamingMode, "hybrid")
	}
	if cfg.TokenTracking.EstimationCharsPerToken != 4 {
		t.Errorf("TokenTracking.EstimationCharsPerToken default = %d, want %d", cfg.TokenTracking.EstimationCharsPerToken, 4)
	}

	if cfg.RateLimits.CleanupInterval != "1h" {
		t.Errorf("RateLimits.CleanupInterval default = %q, want %q", cfg.RateLimits.CleanupInterval, "1h")
	}
	if cfg.RateLimits.Window5hDuration != "5h" {
		t.Errorf("RateLimits.Window5hDuration default = %q, want %q", cfg.RateLimits.Window5hDuration, "5h")
	}

	if cfg.RequestID.HeaderName != "X-Request-ID" {
		t.Errorf("RequestID.HeaderName default = %q, want %q", cfg.RequestID.HeaderName, "X-Request-ID")
	}
}

func TestApplyDefaults_ProviderDefaults(t *testing.T) {
	cfg := &Config{
		Providers: []ProviderConfig{
			{
				Name:    "test",
				APIBase: "https://api.test.com",
				APIKeys: []AccountKeyConfig{
					{Key: "key1"},
				},
			},
		},
	}
	ApplyDefaults(cfg)

	if cfg.Providers[0].Timeout != "30s" {
		t.Errorf("Provider.Timeout default = %q, want %q", cfg.Providers[0].Timeout, "30s")
	}
	if cfg.Providers[0].StreamTimeout != "120s" {
		t.Errorf("Provider.StreamTimeout default = %q, want %q", cfg.Providers[0].StreamTimeout, "120s")
	}
	if cfg.Providers[0].Retry.MaxRetries != 3 {
		t.Errorf("Provider.Retry.MaxRetries default = %d, want %d", cfg.Providers[0].Retry.MaxRetries, 3)
	}
	if cfg.Providers[0].Retry.InitialWait != "1s" {
		t.Errorf("Provider.Retry.InitialWait default = %q, want %q", cfg.Providers[0].Retry.InitialWait, "1s")
	}
	if cfg.Providers[0].Retry.MaxWait != "30s" {
		t.Errorf("Provider.Retry.MaxWait default = %q, want %q", cfg.Providers[0].Retry.MaxWait, "30s")
	}
	if cfg.Providers[0].Retry.Multiplier != 2.0 {
		t.Errorf("Provider.Retry.Multiplier default = %f, want %f", cfg.Providers[0].Retry.Multiplier, 2.0)
	}
	if cfg.Providers[0].CircuitBreaker.Threshold != 5 {
		t.Errorf("Provider.CircuitBreaker.Threshold default = %d, want %d", cfg.Providers[0].CircuitBreaker.Threshold, 5)
	}
	if cfg.Providers[0].CircuitBreaker.Timeout != "60s" {
		t.Errorf("Provider.CircuitBreaker.Timeout default = %q, want %q", cfg.Providers[0].CircuitBreaker.Timeout, "60s")
	}
	if cfg.Providers[0].CircuitBreaker.HalfOpenRequests != 1 {
		t.Errorf("Provider.CircuitBreaker.HalfOpenRequests default = %d, want %d", cfg.Providers[0].CircuitBreaker.HalfOpenRequests, 1)
	}
	if cfg.Providers[0].APIKeys[0].Weight != 1 {
		t.Errorf("APIKey.Weight default = %d, want %d", cfg.Providers[0].APIKeys[0].Weight, 1)
	}
	if !cfg.Providers[0].APIKeys[0].IsEnabled {
		t.Errorf("APIKey.IsEnabled default = %v, want %v", cfg.Providers[0].APIKeys[0].IsEnabled, true)
	}
}

func TestApplyEnvironmentOverrides(t *testing.T) {
	tests := []struct {
		name      string
		envVars   map[string]string
		checkFunc func(*testing.T, *Config)
	}{
		{
			name:    "server host override",
			envVars: map[string]string{"AIPROXY_SERVER_HOST": "192.168.1.1"},
			checkFunc: func(t *testing.T, cfg *Config) {
				if cfg.Server.Host != "192.168.1.1" {
					t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "192.168.1.1")
				}
			},
		},
		{
			name:    "server port override",
			envVars: map[string]string{"AIPROXY_SERVER_PORT": "9999"},
			checkFunc: func(t *testing.T, cfg *Config) {
				if cfg.Server.Port != 9999 {
					t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, 9999)
				}
			},
		},
		{
			name:    "logging level override",
			envVars: map[string]string{"AIPROXY_LOGGING_LEVEL": "debug"},
			checkFunc: func(t *testing.T, cfg *Config) {
				if cfg.Logging.Level != "debug" {
					t.Errorf("Logging.Level = %q, want %q", cfg.Logging.Level, "debug")
				}
			},
		},
		{
			name:    "auth enabled override",
			envVars: map[string]string{"AIPROXY_AUTH_ENABLED": "true"},
			checkFunc: func(t *testing.T, cfg *Config) {
				if !cfg.Auth.Enabled {
					t.Errorf("Auth.Enabled = %v, want %v", cfg.Auth.Enabled, true)
				}
			},
		},
		{
			name:    "auth api keys json override",
			envVars: map[string]string{"AIPROXY_AUTH_API_KEYS": `["key1","key2"]`},
			checkFunc: func(t *testing.T, cfg *Config) {
				if len(cfg.Auth.APIKeys) != 2 {
					t.Errorf("len(Auth.APIKeys) = %d, want %d", len(cfg.Auth.APIKeys), 2)
				}
			},
		},
		{
			name:    "auth api keys comma override",
			envVars: map[string]string{"AIPROXY_AUTH_API_KEYS": "key1,key2,key3"},
			checkFunc: func(t *testing.T, cfg *Config) {
				if len(cfg.Auth.APIKeys) != 3 {
					t.Errorf("len(Auth.APIKeys) = %d, want %d", len(cfg.Auth.APIKeys), 3)
				}
			},
		},
		{
			name:    "metrics enabled override",
			envVars: map[string]string{"AIPROXY_METRICS_ENABLED": "true"},
			checkFunc: func(t *testing.T, cfg *Config) {
				if !cfg.Metrics.Enabled {
					t.Errorf("Metrics.Enabled = %v, want %v", cfg.Metrics.Enabled, true)
				}
			},
		},
		{
			name: "multiple overrides",
			envVars: map[string]string{
				"AIPROXY_SERVER_PORT":    "7777",
				"AIPROXY_DATABASE_PATH":  "/custom/path.db",
				"AIPROXY_LOGGING_LEVEL":  "warn",
				"AIPROXY_ADMIN_ENABLED":  "true",
				"AIPROXY_ADMIN_API_KEYS": `["admin-key"]`,
			},
			checkFunc: func(t *testing.T, cfg *Config) {
				if cfg.Server.Port != 7777 {
					t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, 7777)
				}
				if cfg.Database.Path != "/custom/path.db" {
					t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, "/custom/path.db")
				}
				if cfg.Logging.Level != "warn" {
					t.Errorf("Logging.Level = %q, want %q", cfg.Logging.Level, "warn")
				}
				if !cfg.Admin.Enabled {
					t.Errorf("Admin.Enabled = %v, want %v", cfg.Admin.Enabled, true)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.envVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}

			cfg := &Config{}
			ApplyDefaults(cfg)
			ApplyEnvironmentOverrides(cfg)
			tt.checkFunc(t, cfg)
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  func() *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: func() *Config {
				cfg := &Config{
					Database: DatabaseConfig{Path: "test.db"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "https://api.test.com",
							APIKeys: []AccountKeyConfig{
								{Key: "key1", Weight: 1, IsEnabled: true},
							},
						},
					},
				}
				ApplyDefaults(cfg)
				return cfg
			},
			wantErr: false,
		},
		{
			name: "invalid port",
			config: func() *Config {
				cfg := &Config{
					Server:   ServerConfig{Port: -1},
					Database: DatabaseConfig{Path: "test.db"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "https://api.test.com",
							APIKeys: []AccountKeyConfig{{Key: "key1"}},
						},
					},
				}
				ApplyDefaults(cfg)
				cfg.Server.Port = -1
				return cfg
			},
			wantErr: true,
			errMsg:  "server.port",
		},
		{
			name: "invalid port too high",
			config: func() *Config {
				cfg := &Config{
					Server:   ServerConfig{Port: 70000},
					Database: DatabaseConfig{Path: "test.db"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "https://api.test.com",
							APIKeys: []AccountKeyConfig{{Key: "key1"}},
						},
					},
				}
				return cfg
			},
			wantErr: true,
			errMsg:  "server.port",
		},
		{
			name: "missing database path",
			config: func() *Config {
				cfg := &Config{
					Database: DatabaseConfig{Path: ""},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "https://api.test.com",
							APIKeys: []AccountKeyConfig{{Key: "key1"}},
						},
					},
				}
				return cfg
			},
			wantErr: true,
			errMsg:  "database.path",
		},
		{
			name: "invalid logging level",
			config: func() *Config {
				cfg := &Config{
					Logging:  LoggingConfig{Level: "invalid", Format: "json", Output: "stdout"},
					Database: DatabaseConfig{Path: "test.db"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "https://api.test.com",
							APIKeys: []AccountKeyConfig{{Key: "key1"}},
						},
					},
				}
				ApplyDefaults(cfg)
				cfg.Logging.Level = "invalid"
				return cfg
			},
			wantErr: true,
			errMsg:  "logging.level",
		},
		{
			name: "invalid logging format",
			config: func() *Config {
				cfg := &Config{
					Logging:  LoggingConfig{Level: "info", Format: "invalid", Output: "stdout"},
					Database: DatabaseConfig{Path: "test.db", JournalMode: "WAL", AutoVacuum: "INCREMENTAL"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "https://api.test.com",
							APIKeys: []AccountKeyConfig{{Key: "key1"}},
						},
					},
				}
				return cfg
			},
			wantErr: true,
			errMsg:  "logging.format",
		},
		{
			name: "auth enabled without api keys",
			config: func() *Config {
				cfg := &Config{
					Auth:     AuthConfig{Enabled: true, APIKeys: []string{}},
					Database: DatabaseConfig{Path: "test.db"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "https://api.test.com",
							APIKeys: []AccountKeyConfig{{Key: "key1"}},
						},
					},
				}
				ApplyDefaults(cfg)
				cfg.Auth.APIKeys = []string{}
				return cfg
			},
			wantErr: true,
			errMsg:  "auth.api_keys",
		},
		{
			name: "no providers",
			config: func() *Config {
				cfg := &Config{
					Database:  DatabaseConfig{Path: "test.db"},
					Providers: []ProviderConfig{},
				}
				ApplyDefaults(cfg)
				return cfg
			},
			wantErr: true,
			errMsg:  "providers",
		},
		{
			name: "provider missing name",
			config: func() *Config {
				cfg := &Config{
					Database: DatabaseConfig{Path: "test.db"},
					Providers: []ProviderConfig{
						{
							Name:    "",
							APIBase: "https://api.test.com",
							APIKeys: []AccountKeyConfig{{Key: "key1"}},
						},
					},
				}
				ApplyDefaults(cfg)
				cfg.Providers[0].Name = ""
				return cfg
			},
			wantErr: true,
			errMsg:  "providers[0].name",
		},
		{
			name: "provider missing api base",
			config: func() *Config {
				cfg := &Config{
					Logging:  LoggingConfig{Level: "info", Format: "json", Output: "stdout"},
					Auth:     AuthConfig{HeaderName: "Authorization"},
					Database: DatabaseConfig{Path: "test.db", JournalMode: "WAL", AutoVacuum: "INCREMENTAL"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "",
							APIKeys: []AccountKeyConfig{{Key: "key1"}},
						},
					},
				}
				return cfg
			},
			wantErr: true,
			errMsg:  "providers[0].api_base",
		},
		{
			name: "provider invalid api base",
			config: func() *Config {
				cfg := &Config{
					Logging:   LoggingConfig{Level: "info", Format: "json", Output: "stdout"},
					Auth:      AuthConfig{HeaderName: "Authorization"},
					RequestID: RequestIDConfig{HeaderName: "X-Request-ID"},
					Database:  DatabaseConfig{Path: "test.db", JournalMode: "WAL", AutoVacuum: "INCREMENTAL"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "://invalid",
							APIKeys: []AccountKeyConfig{{Key: "key1"}},
							Retry:   RetryConfig{Multiplier: 1.0},
						},
					},
				}
				return cfg
			},
			wantErr: true,
			errMsg:  "providers[0].api_base",
		},
		{
			name: "provider missing api keys",
			config: func() *Config {
				cfg := &Config{
					Logging:  LoggingConfig{Level: "info", Format: "json", Output: "stdout"},
					Auth:     AuthConfig{HeaderName: "Authorization"},
					Database: DatabaseConfig{Path: "test.db", JournalMode: "WAL", AutoVacuum: "INCREMENTAL"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "https://api.test.com",
							APIKeys: []AccountKeyConfig{},
						},
					},
				}
				ApplyDefaults(cfg)
				cfg.Providers[0].APIKeys = []AccountKeyConfig{}
				return cfg
			},
			wantErr: true,
			errMsg:  "providers[0].api_keys",
		},
		{
			name: "api key missing key",
			config: func() *Config {
				cfg := &Config{
					Logging:  LoggingConfig{Level: "info", Format: "json", Output: "stdout"},
					Auth:     AuthConfig{HeaderName: "Authorization"},
					Database: DatabaseConfig{Path: "test.db", JournalMode: "WAL", AutoVacuum: "INCREMENTAL"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "https://api.test.com",
							APIKeys: []AccountKeyConfig{{Key: ""}},
						},
					},
				}
				ApplyDefaults(cfg)
				cfg.Providers[0].APIKeys[0].Key = ""
				return cfg
			},
			wantErr: true,
			errMsg:  "providers[0].api_keys[0].key",
		},
		{
			name: "api key negative weight",
			config: func() *Config {
				cfg := &Config{
					Logging:  LoggingConfig{Level: "info", Format: "json", Output: "stdout"},
					Auth:     AuthConfig{HeaderName: "Authorization"},
					Database: DatabaseConfig{Path: "test.db", JournalMode: "WAL", AutoVacuum: "INCREMENTAL"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "https://api.test.com",
							APIKeys: []AccountKeyConfig{{Key: "key1", Weight: -1}},
						},
					},
				}
				ApplyDefaults(cfg)
				cfg.Providers[0].APIKeys[0].Weight = -1
				return cfg
			},
			wantErr: true,
			errMsg:  "providers[0].api_keys[0].weight",
		},
		{
			name: "duplicate provider names",
			config: func() *Config {
				cfg := &Config{
					Logging:  LoggingConfig{Level: "info", Format: "json", Output: "stdout"},
					Auth:     AuthConfig{HeaderName: "Authorization"},
					Database: DatabaseConfig{Path: "test.db", JournalMode: "WAL", AutoVacuum: "INCREMENTAL"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "https://api.test.com",
							APIKeys: []AccountKeyConfig{{Key: "key1"}},
						},
						{
							Name:    "test",
							APIBase: "https://api.test2.com",
							APIKeys: []AccountKeyConfig{{Key: "key2"}},
						},
					},
				}
				ApplyDefaults(cfg)
				cfg.Providers[1].Name = "test"
				return cfg
			},
			wantErr: true,
			errMsg:  "providers[1].name",
		},
		{
			name: "multiple default providers",
			config: func() *Config {
				cfg := &Config{
					Logging:  LoggingConfig{Level: "info", Format: "json", Output: "stdout"},
					Auth:     AuthConfig{HeaderName: "Authorization"},
					Database: DatabaseConfig{Path: "test.db", JournalMode: "WAL", AutoVacuum: "INCREMENTAL"},
					Providers: []ProviderConfig{
						{
							Name:      "test1",
							APIBase:   "https://api.test.com",
							IsDefault: true,
							APIKeys:   []AccountKeyConfig{{Key: "key1"}},
						},
						{
							Name:      "test2",
							APIBase:   "https://api.test2.com",
							IsDefault: true,
							APIKeys:   []AccountKeyConfig{{Key: "key2"}},
						},
					},
				}
				ApplyDefaults(cfg)
				cfg.Providers[0].IsDefault = true
				cfg.Providers[1].IsDefault = true
				return cfg
			},
			wantErr: true,
			errMsg:  "only one provider can be default",
		},
		{
			name: "fallback enabled without providers",
			config: func() *Config {
				cfg := &Config{
					Logging:  LoggingConfig{Level: "info", Format: "json", Output: "stdout"},
					Auth:     AuthConfig{HeaderName: "Authorization"},
					Fallback: FallbackConfig{Enabled: true, Providers: []string{}},
					Database: DatabaseConfig{Path: "test.db", JournalMode: "WAL", AutoVacuum: "INCREMENTAL"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "https://api.test.com",
							APIKeys: []AccountKeyConfig{{Key: "key1"}},
						},
					},
				}
				ApplyDefaults(cfg)
				cfg.Fallback.Enabled = true
				cfg.Fallback.Providers = []string{}
				return cfg
			},
			wantErr: true,
			errMsg:  "fallback.providers",
		},
		{
			name: "fallback invalid strategy",
			config: func() *Config {
				cfg := &Config{
					Logging:  LoggingConfig{Level: "info", Format: "json", Output: "stdout"},
					Auth:     AuthConfig{HeaderName: "Authorization"},
					Fallback: FallbackConfig{Enabled: true, Strategy: "invalid", Providers: []string{"test"}},
					Database: DatabaseConfig{Path: "test.db", JournalMode: "WAL", AutoVacuum: "INCREMENTAL"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "https://api.test.com",
							APIKeys: []AccountKeyConfig{{Key: "key1"}},
						},
					},
				}
				ApplyDefaults(cfg)
				cfg.Fallback.Strategy = "invalid"
				return cfg
			},
			wantErr: true,
			errMsg:  "fallback.strategy",
		},
		{
			name: "admin enabled without api keys",
			config: func() *Config {
				cfg := &Config{
					Logging:  LoggingConfig{Level: "info", Format: "json", Output: "stdout"},
					Auth:     AuthConfig{HeaderName: "Authorization"},
					Admin:    AdminConfig{Enabled: true, APIKeys: []string{}, Listen: "127.0.0.1:8081"},
					Database: DatabaseConfig{Path: "test.db", JournalMode: "WAL", AutoVacuum: "INCREMENTAL"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "https://api.test.com",
							APIKeys: []AccountKeyConfig{{Key: "key1"}},
						},
					},
				}
				ApplyDefaults(cfg)
				cfg.Admin.Enabled = true
				cfg.Admin.APIKeys = []string{}
				return cfg
			},
			wantErr: true,
			errMsg:  "admin.api_keys",
		},
		{
			name: "invalid retry multiplier",
			config: func() *Config {
				cfg := &Config{
					Database: DatabaseConfig{Path: "test.db"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "https://api.test.com",
							APIKeys: []AccountKeyConfig{{Key: "key1"}},
							Retry:   RetryConfig{Multiplier: 0.5},
						},
					},
				}
				ApplyDefaults(cfg)
				cfg.Providers[0].Retry.Multiplier = 0.5
				return cfg
			},
			wantErr: true,
			errMsg:  "providers[0].retry.multiplier",
		},
		{
			name: "invalid account limits negative rpm",
			config: func() *Config {
				rpm := -10
				cfg := &Config{
					Logging:   LoggingConfig{Level: "info", Format: "json", Output: "stdout"},
					Auth:      AuthConfig{HeaderName: "Authorization"},
					RequestID: RequestIDConfig{HeaderName: "X-Request-ID"},
					Database:  DatabaseConfig{Path: "test.db", JournalMode: "WAL", AutoVacuum: "INCREMENTAL"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "https://api.test.com",
							APIKeys: []AccountKeyConfig{
								{
									Key: "key1",
									Limits: &domain.AccountLimits{
										RPM: &rpm,
									},
								},
							},
							Retry: RetryConfig{Multiplier: 1.0},
						},
					},
				}
				return cfg
			},
			wantErr: true,
			errMsg:  "providers[0].api_keys[0].limits.rpm",
		},
		{
			name: "invalid token tracking mode",
			config: func() *Config {
				cfg := &Config{
					Logging:       LoggingConfig{Level: "info", Format: "json", Output: "stdout"},
					Auth:          AuthConfig{HeaderName: "Authorization"},
					RequestID:     RequestIDConfig{HeaderName: "X-Request-ID"},
					TokenTracking: TokenTrackingConfig{Enabled: true, StreamingMode: "invalid", EstimationCharsPerToken: 4},
					Database:      DatabaseConfig{Path: "test.db", JournalMode: "WAL", AutoVacuum: "INCREMENTAL"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "https://api.test.com",
							APIKeys: []AccountKeyConfig{{Key: "key1"}},
							Retry:   RetryConfig{Multiplier: 1.0},
						},
					},
				}
				return cfg
			},
			wantErr: true,
			errMsg:  "token_tracking.streaming_mode",
		},
		{
			name: "invalid token tracking estimation",
			config: func() *Config {
				cfg := &Config{
					Logging:       LoggingConfig{Level: "info", Format: "json", Output: "stdout"},
					Auth:          AuthConfig{HeaderName: "Authorization"},
					RequestID:     RequestIDConfig{HeaderName: "X-Request-ID"},
					TokenTracking: TokenTrackingConfig{Enabled: true, StreamingMode: "hybrid", EstimationCharsPerToken: 0},
					Database:      DatabaseConfig{Path: "test.db", JournalMode: "WAL", AutoVacuum: "INCREMENTAL"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "https://api.test.com",
							APIKeys: []AccountKeyConfig{{Key: "key1"}},
							Retry:   RetryConfig{Multiplier: 1.0},
						},
					},
				}
				return cfg
			},
			wantErr: true,
			errMsg:  "token_tracking.estimation_chars_per_token",
		},
		{
			name: "invalid journal mode",
			config: func() *Config {
				cfg := &Config{
					Database: DatabaseConfig{Path: "test.db", JournalMode: "INVALID"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "https://api.test.com",
							APIKeys: []AccountKeyConfig{{Key: "key1"}},
						},
					},
				}
				return cfg
			},
			wantErr: true,
			errMsg:  "database.journal_mode",
		},
		{
			name: "invalid auto vacuum",
			config: func() *Config {
				cfg := &Config{
					Database: DatabaseConfig{Path: "test.db", JournalMode: "WAL", AutoVacuum: "INVALID"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "https://api.test.com",
							APIKeys: []AccountKeyConfig{{Key: "key1"}},
						},
					},
				}
				return cfg
			},
			wantErr: true,
			errMsg:  "database.auto_vacuum",
		},
		{
			name: "invalid server read timeout",
			config: func() *Config {
				cfg := &Config{
					Server:   ServerConfig{ReadTimeout: "invalid"},
					Database: DatabaseConfig{Path: "test.db"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "https://api.test.com",
							APIKeys: []AccountKeyConfig{{Key: "key1"}},
						},
					},
				}
				return cfg
			},
			wantErr: true,
			errMsg:  "server.read_timeout",
		},
		{
			name: "invalid provider timeout",
			config: func() *Config {
				cfg := &Config{
					Logging:  LoggingConfig{Level: "info", Format: "json", Output: "stdout"},
					Auth:     AuthConfig{HeaderName: "Authorization"},
					Database: DatabaseConfig{Path: "test.db", JournalMode: "WAL", AutoVacuum: "INCREMENTAL"},
					Providers: []ProviderConfig{
						{
							Name:    "test",
							APIBase: "https://api.test.com",
							Timeout: "invalid",
							APIKeys: []AccountKeyConfig{{Key: "key1"}},
						},
					},
				}
				return cfg
			},
			wantErr: true,
			errMsg:  "providers[0].timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.config()
			err := Validate(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !containsString(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, should contain %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestLoadExampleConfig(t *testing.T) {
	examplePath := filepath.Join("..", "config", "config.example.json")
	if _, err := os.Stat(examplePath); os.IsNotExist(err) {
		t.Skip("Example config file not found")
	}

	cfg, err := Load(examplePath)
	if err != nil {
		t.Fatalf("Failed to load example config: %v", err)
	}

	if err := Validate(cfg); err != nil {
		t.Errorf("Example config validation failed: %v", err)
	}

	if len(cfg.Providers) == 0 {
		t.Error("Example config should have at least one provider")
	}
}

func TestConfigJSONTags(t *testing.T) {
	jsonData := `{
		"server": {
			"host": "0.0.0.0",
			"port": 8080,
			"read_timeout": "30s",
			"write_timeout": "120s",
			"idle_timeout": "120s",
			"graceful_shutdown_timeout": "30s",
			"max_request_body_size": 10485760
		},
		"database": {
			"path": "data/test.db",
			"busy_timeout": 5000,
			"journal_mode": "WAL",
			"cache_size": -64000,
			"auto_vacuum": "INCREMENTAL"
		},
		"logging": {
			"level": "debug",
			"format": "json",
			"output": "stdout",
			"include_request_body": true,
			"include_response_body": true
		},
		"auth": {
			"enabled": true,
			"api_keys": ["key1", "key2"],
			"header_name": "X-API-Key",
			"key_prefix": ""
		},
		"providers": [
			{
				"name": "test-provider",
				"api_base": "https://api.test.com/v1",
				"models": ["model1", "model2"],
				"is_default": true,
				"is_enabled": true,
				"headers": {"X-Custom": "value"},
				"timeout": "30s",
				"stream_timeout": "120s",
				"api_keys": [
					{
						"key": "test-key",
						"name": "primary",
						"weight": 2,
						"priority": 1,
						"is_enabled": true,
						"limits": {
							"rpm": 20,
							"daily": 1000
						}
					}
				],
				"retry": {
					"max_retries": 3,
					"initial_wait": "1s",
					"max_wait": "30s",
					"multiplier": 2.0
				},
				"circuit_breaker": {
					"threshold": 5,
					"timeout": "60s",
					"half_open_requests": 1
				}
			}
		],
		"model_mapping": {
			"gpt-4": "openai/gpt-4o"
		},
		"fallback": {
			"enabled": true,
			"strategy": "sequential",
			"providers": ["test-provider"]
		},
		"admin": {
			"enabled": true,
			"listen": "127.0.0.1:8081",
			"api_keys": ["admin-key"],
			"rate_limit": 100
		},
		"metrics": {
			"enabled": true,
			"prometheus": {
				"enabled": true,
				"path": "/metrics"
			},
			"json": {
				"enabled": true
			},
			"namespace": "aiproxy"
		},
		"token_tracking": {
			"enabled": true,
			"streaming_mode": "hybrid",
			"estimation_chars_per_token": 4,
			"reconciliation_interval": "5m"
		},
		"rate_limits": {
			"cleanup_interval": "1h",
			"window_5h_duration": "5h"
		},
		"request_id": {
			"header_name": "X-Request-ID",
			"generate_if_missing": true
		}
	}`

	cfg, err := LoadFromBytes([]byte(jsonData))
	if err != nil {
		t.Fatalf("LoadFromBytes() error = %v", err)
	}

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "0.0.0.0")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, 8080)
	}
	if cfg.Server.ReadTimeout != "30s" {
		t.Errorf("Server.ReadTimeout = %q, want %q", cfg.Server.ReadTimeout, "30s")
	}
	if cfg.Database.Path != "data/test.db" {
		t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, "data/test.db")
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Logging.Level = %q, want %q", cfg.Logging.Level, "debug")
	}
	if !cfg.Logging.IncludeRequestBody {
		t.Errorf("Logging.IncludeRequestBody = %v, want %v", cfg.Logging.IncludeRequestBody, true)
	}
	if !cfg.Auth.Enabled {
		t.Errorf("Auth.Enabled = %v, want %v", cfg.Auth.Enabled, true)
	}
	if len(cfg.Auth.APIKeys) != 2 {
		t.Errorf("len(Auth.APIKeys) = %d, want %d", len(cfg.Auth.APIKeys), 2)
	}
	if cfg.Auth.HeaderName != "X-API-Key" {
		t.Errorf("Auth.HeaderName = %q, want %q", cfg.Auth.HeaderName, "X-API-Key")
	}
	if len(cfg.Providers) != 1 {
		t.Errorf("len(Providers) = %d, want %d", len(cfg.Providers), 1)
	}
	if cfg.Providers[0].Name != "test-provider" {
		t.Errorf("Providers[0].Name = %q, want %q", cfg.Providers[0].Name, "test-provider")
	}
	if len(cfg.Providers[0].Models) != 2 {
		t.Errorf("len(Providers[0].Models) = %d, want %d", len(cfg.Providers[0].Models), 2)
	}
	if !cfg.Providers[0].IsDefault {
		t.Errorf("Providers[0].IsDefault = %v, want %v", cfg.Providers[0].IsDefault, true)
	}
	if len(cfg.Providers[0].APIKeys) != 1 {
		t.Errorf("len(Providers[0].APIKeys) = %d, want %d", len(cfg.Providers[0].APIKeys), 1)
	}
	if cfg.Providers[0].APIKeys[0].Limits == nil {
		t.Error("Providers[0].APIKeys[0].Limits should not be nil")
	}
	if cfg.Providers[0].APIKeys[0].Limits.RPM == nil || *cfg.Providers[0].APIKeys[0].Limits.RPM != 20 {
		t.Errorf("Providers[0].APIKeys[0].Limits.RPM = %v, want 20", cfg.Providers[0].APIKeys[0].Limits.RPM)
	}
	if cfg.ModelMapping["gpt-4"] != "openai/gpt-4o" {
		t.Errorf("ModelMapping[\"gpt-4\"] = %q, want %q", cfg.ModelMapping["gpt-4"], "openai/gpt-4o")
	}
	if !cfg.Fallback.Enabled {
		t.Errorf("Fallback.Enabled = %v, want %v", cfg.Fallback.Enabled, true)
	}
	if cfg.Fallback.Strategy != "sequential" {
		t.Errorf("Fallback.Strategy = %q, want %q", cfg.Fallback.Strategy, "sequential")
	}
	if !cfg.Admin.Enabled {
		t.Errorf("Admin.Enabled = %v, want %v", cfg.Admin.Enabled, true)
	}
	if cfg.Admin.Listen != "127.0.0.1:8081" {
		t.Errorf("Admin.Listen = %q, want %q", cfg.Admin.Listen, "127.0.0.1:8081")
	}
	if !cfg.Metrics.Enabled {
		t.Errorf("Metrics.Enabled = %v, want %v", cfg.Metrics.Enabled, true)
	}
	if !cfg.Metrics.Prometheus.Enabled {
		t.Errorf("Metrics.Prometheus.Enabled = %v, want %v", cfg.Metrics.Prometheus.Enabled, true)
	}
	if !cfg.TokenTracking.Enabled {
		t.Errorf("TokenTracking.Enabled = %v, want %v", cfg.TokenTracking.Enabled, true)
	}
	if cfg.RequestID.HeaderName != "X-Request-ID" {
		t.Errorf("RequestID.HeaderName = %q, want %q", cfg.RequestID.HeaderName, "X-Request-ID")
	}
	if !cfg.RequestID.GenerateIfMissing {
		t.Errorf("RequestID.GenerateIfMissing = %v, want %v", cfg.RequestID.GenerateIfMissing, true)
	}
}

func TestDurationParsing(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			ReadTimeout: "45s",
		},
		Database: DatabaseConfig{Path: "test.db"},
		Providers: []ProviderConfig{
			{
				Name:          "test",
				APIBase:       "https://api.test.com",
				Timeout:       "45s",
				StreamTimeout: "90s",
				Retry: RetryConfig{
					InitialWait: "2s",
					MaxWait:     "60s",
				},
				CircuitBreaker: CircuitBreakerConfig{
					Timeout: "120s",
				},
				APIKeys: []AccountKeyConfig{{Key: "key1"}},
			},
		},
		TokenTracking: TokenTrackingConfig{
			ReconciliationInterval: "10m",
		},
		RateLimits: RateLimitsConfig{
			CleanupInterval:  "2h",
			Window5hDuration: "6h",
		},
	}
	ApplyDefaults(cfg)

	if cfg.Server.ReadTimeout != "45s" {
		t.Errorf("Server.ReadTimeout = %q, want %q", cfg.Server.ReadTimeout, "45s")
	}
}

func TestEmptyAPIKeyIsEnabled(t *testing.T) {
	cfg := &Config{
		Database: DatabaseConfig{Path: "test.db"},
		Providers: []ProviderConfig{
			{
				Name:    "test",
				APIBase: "https://api.test.com",
				APIKeys: []AccountKeyConfig{
					{Key: "", IsEnabled: false},
					{Key: "key1", IsEnabled: false},
				},
			},
		},
	}
	ApplyDefaults(cfg)

	if cfg.Providers[0].APIKeys[0].IsEnabled {
		t.Error("Empty key should remain disabled")
	}
	if !cfg.Providers[0].APIKeys[1].IsEnabled {
		t.Error("Non-empty key should be enabled after ApplyDefaults")
	}
}

func TestProviderTimeoutValidation(t *testing.T) {
	tests := []struct {
		name     string
		timeout  string
		streamTo string
		wantErr  bool
	}{
		{"valid durations", "30s", "120s", false},
		{"invalid timeout", "invalid", "120s", true},
		{"invalid stream_timeout", "30s", "invalid", true},
		{"empty durations", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Database: DatabaseConfig{Path: "test.db"},
				Providers: []ProviderConfig{
					{
						Name:          "test",
						APIBase:       "https://api.test.com",
						Timeout:       tt.timeout,
						StreamTimeout: tt.streamTo,
						APIKeys:       []AccountKeyConfig{{Key: "key1"}},
					},
				},
			}
			ApplyDefaults(cfg)
			if tt.timeout != "" {
				cfg.Providers[0].Timeout = tt.timeout
			}
			if tt.streamTo != "" {
				cfg.Providers[0].StreamTimeout = tt.streamTo
			}
			err := Validate(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > 0 && containsStringHelper(s, substr)))
}

func containsStringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestMarshalUnmarshalConfig(t *testing.T) {
	rpm := 20
	daily := 1000

	original := &Config{
		Server: ServerConfig{
			Host:        "localhost",
			Port:        8080,
			ReadTimeout: "30s",
		},
		Database: DatabaseConfig{
			Path:        "data/test.db",
			BusyTimeout: 5000,
			JournalMode: "WAL",
		},
		Providers: []ProviderConfig{
			{
				Name:    "test",
				APIBase: "https://api.test.com",
				APIKeys: []AccountKeyConfig{
					{
						Key:       "key1",
						Name:      "primary",
						Weight:    1,
						Priority:  1,
						IsEnabled: true,
						Limits: &domain.AccountLimits{
							RPM:   &rpm,
							Daily: &daily,
						},
					},
				},
			},
		},
	}

	data, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	parsed, err := LoadFromBytes(data)
	if err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	if parsed.Server.Host != original.Server.Host {
		t.Errorf("Server.Host mismatch: got %q, want %q", parsed.Server.Host, original.Server.Host)
	}
	if parsed.Server.Port != original.Server.Port {
		t.Errorf("Server.Port mismatch: got %d, want %d", parsed.Server.Port, original.Server.Port)
	}
	if len(parsed.Providers) != len(original.Providers) {
		t.Errorf("Providers count mismatch: got %d, want %d", len(parsed.Providers), len(original.Providers))
	}
}
