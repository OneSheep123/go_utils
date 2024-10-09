package ratelimit

import (
	"sync"
	"time"
)

type TokenBucketV1 struct {
	mu          sync.Mutex
	capacity    int64 // 桶的最大容量
	tokens      int64 // 当前令牌数
	rate        int64 // 每秒生成的令牌数
	lastUpdated time.Time
}

func NewTokenBucketV1(capacity, rate int64) *TokenBucketV1 {
	return &TokenBucketV1{
		capacity:    capacity,
		tokens:      capacity, // 初始化时满桶
		rate:        rate,
		lastUpdated: time.Now(),
	}
}

// Consume 尝试消费指定数量的令牌
func (tb *TokenBucketV1) Consume(tokens int64) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// 计算自从上次更新以来应该添加的令牌数量
	tokenToAdd := tb.rate * int64(time.Since(tb.lastUpdated).Seconds())
	if tokenToAdd > 0 {
		tb.tokens = min(tb.capacity, tb.tokens+tokenToAdd)
		tb.lastUpdated = time.Now()
	}

	if tb.tokens < tokens {
		return false // 不足，拒绝请求
	}

	tb.tokens -= tokens
	return true // 允许请求
}

// Tokens 返回还剩余多少令牌
func (tb *TokenBucketV1) Tokens() int64 {
	return tb.tokens
}
