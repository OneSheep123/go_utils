package ratelimit

import (
	"github.com/redis/go-redis/v9"
	ratelimit2 "go_utils/ratelimit"
	"sync/atomic"
)

func NewLocalCntLimiter(cmd redis.Cmdable, maxActive int64) ratelimit2.Limiter {
	mc := atomic.Int64{}
	mc.Store(maxActive)
	return &ratelimit2.LocalCnt{
		MaxActive: &mc,
	}
}
