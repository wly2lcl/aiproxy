package limiter

import (
	"context"

	"github.com/wangluyao/aiproxy/internal/domain"
)

type Limiter interface {
	Allow(ctx context.Context, key string) (bool, error)
	Record(ctx context.Context, key string, delta int) error
	GetState(ctx context.Context, key string) (*domain.LimitState, error)
	Reset(ctx context.Context, key string) error
	LimitType() domain.LimitType
}
