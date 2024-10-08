package cache

import (
	"context"
	"time"
)

type Cache interface {
	Get(ctx context.Context, key string) (any, error)
	Set(ctx context.Context, key string, value any, expireTime time.Duration) error
	Delete(ctx context.Context, key string) error
	LoadAndDelete(ctx context.Context, key string) (any, error)
}
