package storage

import (
	"context"
	"time"

	"github.com/wangluyao/aiproxy/internal/domain"
)

type TokenUsage struct {
	RequestID        string
	AccountID        string
	ProviderID       string
	Model            string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	IsStreaming      bool
	IsEstimated      bool
	EstimatedAt      *time.Time
	CorrectedAt      *time.Time
}

type TokenUsageSummary struct {
	RequestCount          int
	TotalPromptTokens     int
	TotalCompletionTokens int
	TotalTokens           int
	StreamingCount        int
	EstimatedCount        int
}

type Storage interface {
	UpsertProvider(ctx context.Context, provider *domain.Provider) error
	GetProvider(ctx context.Context, id string) (*domain.Provider, error)
	ListProviders(ctx context.Context) ([]*domain.Provider, error)

	UpsertAccount(ctx context.Context, account *domain.Account) error
	GetAccount(ctx context.Context, id string) (*domain.Account, error)
	ListAccounts(ctx context.Context, providerID string) ([]*domain.Account, error)

	GetRateLimit(ctx context.Context, accountID string, limitType domain.LimitType) (*domain.LimitState, error)
	IncrementRateLimit(ctx context.Context, accountID string, limitType domain.LimitType, delta int) error
	ResetRateLimit(ctx context.Context, accountID string, limitType domain.LimitType) error
	UpdateAccountLastUsed(ctx context.Context, accountID string) error
	GetAllAccountLastUsed(ctx context.Context) (map[string]time.Time, error)

	RecordTokenUsage(ctx context.Context, usage *TokenUsage) error
	GetTokenUsage(ctx context.Context, accountID string, since time.Time) (*TokenUsageSummary, error)

	CleanupExpiredRateLimits(ctx context.Context) error

	// Get all rate limit states for an account
	GetAllRateLimits(ctx context.Context, accountID string) ([]*domain.LimitState, error)

	// Get recent request logs
	GetRecentLogs(ctx context.Context, limit int) ([]*RequestLog, error)

	// Get log by request ID with full body
	GetLogByID(ctx context.Context, requestID string) (*RequestLog, error)

	RecordRequestLog(ctx context.Context, log *RequestLog) error

	// Time series data for charts
	GetRequestTimeSeries(ctx context.Context, since time.Time, interval string) ([]*TimeSeriesPoint, error)

	// Account statistics
	GetAllAccountStats(ctx context.Context, since time.Time) ([]*AccountStats, error)

	// Model statistics
	GetModelStats(ctx context.Context, since time.Time) ([]*ModelStats, error)

	// Latency data for percentile calculation
	GetLatencyData(ctx context.Context, since time.Time) ([]*LatencyData, error)

	// Account model usage stats
	GetAccountModelStats(ctx context.Context, accountID string, since time.Time) (map[string]int64, error)

	// API Key management
	CreateAPIKey(ctx context.Context, keyHash, name string, expiresAt *time.Time) (int64, error)
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKey, error)
	ListAPIKeys(ctx context.Context) ([]*APIKey, error)
	UpdateAPIKeyUsage(ctx context.Context, keyHash string) error
	DeleteAPIKey(ctx context.Context, id int64) error
	ToggleAPIKey(ctx context.Context, id int64, enabled bool) error

	Close() error
}

// RequestLog represents a recent request for display
type RequestLog struct {
	RequestID    string
	AccountID    string
	ProviderID   string
	Model        string
	Status       int
	Tokens       int
	TTFTMs       float64
	LatencyMs    float64
	ErrorType    string
	ErrorMessage string
	Timestamp    time.Time
	IsStreaming  bool
	RequestBody  string
	ResponseBody string
}

type APIKey struct {
	ID           int64
	KeyHash      string
	Name         string
	IsEnabled    bool
	CreatedAt    time.Time
	LastUsedAt   *time.Time
	RequestCount int64
	ExpiresAt    *time.Time
}

// TimeSeriesPoint represents a data point for time series charts
type TimeSeriesPoint struct {
	Timestamp time.Time
	Count     int64
	Tokens    int64
	Errors    int64
}

// AccountStats represents statistics for an account
type AccountStats struct {
	AccountID    string
	RequestCount int64
	ErrorCount   int64
	TotalTokens  int64
	AvgLatencyMs float64
	AvgTTFTMs    float64
	SuccessRate  float64
	LastUsedAt   *time.Time
}

// ModelStats represents statistics for a model
type ModelStats struct {
	Model        string
	RequestCount int64
	ErrorCount   int64
	TotalTokens  int64
	AvgTTFTMs    float64
	AvgLatencyMs float64
	SuccessRate  float64
}

type LatencyData struct {
	LatencyMs float64
	TTFTMs    float64
	Status    int
}
