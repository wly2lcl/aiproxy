package limiter

import (
	"context"
	"time"

	"github.com/wangluyao/aiproxy/internal/domain"
)

type Limiter interface {
	Allow(ctx context.Context, key string) (bool, error)
	Record(ctx context.Context, key string, delta int) error
	GetState(ctx context.Context, key string) (*domain.LimitState, error)
	Reset(ctx context.Context, key string) error
	LimitType() domain.LimitType
	// LoadState loads persisted state from database into memory
	// This is used to restore rate limit state after service restart
	LoadState(ctx context.Context, key string, state *domain.LimitState) error
	// CleanupStale removes entries that haven't been accessed for more than maxAge
	// Returns the number of entries removed
	CleanupStale(maxAge time.Duration) int
}
