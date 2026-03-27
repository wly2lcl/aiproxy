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

	RecordTokenUsage(ctx context.Context, usage *TokenUsage) error
	GetTokenUsage(ctx context.Context, accountID string, since time.Time) (*TokenUsageSummary, error)

	CleanupExpiredRateLimits(ctx context.Context) error

	Close() error
}
