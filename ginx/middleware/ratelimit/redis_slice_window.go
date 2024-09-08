package ratelimit

import (
	ratelimit2 "go_utils/ratelimit"
	"time"

	"github.com/redis/go-redis/v9"
)

// NewRedisSlidingWindowLimiter 创建一个基于 redis 的滑动窗口限流器.
// cmd: 可传入 redis 的客户端
// interval: 窗口大小
// rate: 阈值
// 表示: 在 interval 内允许 rate 个请求
// 示例: 1s 内允许 3000 个请求
func NewRedisSlidingWindowLimiter(cmd redis.Cmdable,
	interval time.Duration, rate int) ratelimit2.Limiter {
	return &ratelimit2.RedisSlidingWindowLimiter{
		Cmd:      cmd,
		Interval: interval,
		Rate:     rate,
	}
}
