package ratelimit

import (
	"context"
	"sync/atomic"
)

var _ Limiter = &LocalCnt{}

type LocalCnt struct {
	MaxActive   *atomic.Int64
	CountActive *atomic.Int64
}

func (l *LocalCnt) Limit(ctx context.Context, key string) (bool, error) {
	current := l.CountActive.Add(1)
	defer func() {
		l.CountActive.CompareAndSwap(current, current-1)
	}()
	if current <= l.MaxActive.Load() {
		return true, nil
	}
	return false, nil
}
