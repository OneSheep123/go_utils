package ratelimit

import (
	"github.com/redis/go-redis/v9"
	"go_utils/internal/ratelimit"
	"sync/atomic"
)

func NewLocalCntLimiter(cmd redis.Cmdable, maxActive int64) ratelimit.Limiter {
	mc := atomic.Int64{}
	mc.Store(maxActive)
	return &ratelimit.LocalCnt{
		MaxActive: &mc,
	}
}
