package cache

import (
	"context"
	"errors"
	"fmt"
	"go_utils/internal/errs"
	"sync"
	"time"
)

type item struct {
	value      any
	expireTime time.Time
}

func (i *item) deadlineBefore(t time.Time) bool {
	return !i.expireTime.IsZero() && i.expireTime.Before(t)
}

type BuildInMapCache struct {
	mutex sync.RWMutex
	m     map[string]*item
	close chan struct{}

	// CDC(change data capture)实现: 一个key被更新后进行通知或者操作一些事情
	onEvicted func(key string, value any)
}

// BuildInMapCacheOption option模式
type BuildInMapCacheOption func(cache *BuildInMapCache)

func WithOnEvicted(fn func(key string, val any)) BuildInMapCacheOption {
	return func(cache *BuildInMapCache) {
		cache.onEvicted = fn
	}
}

// NewBuildInMapCache 构建一个本地缓存，采用惰性删除和定期删除
// interval: 定期检查过期键时间
func NewBuildInMapCache(interval time.Duration, opts ...BuildInMapCacheOption) *BuildInMapCache {
	res := &BuildInMapCache{
		m:     map[string]*item{},
		close: make(chan struct{}),
		onEvicted: func(key string, value any) {

		},
	}

	for _, opt := range opts {
		opt(res)
	}

	go func() {
		ticker := time.NewTicker(interval)
		for {
			select {
			case t := <-ticker.C:
				res.mutex.Lock()
				i := 0
				for key, v := range res.m {
					// 随机抽取1000个
					if i > 1000 {
						break
					}
					if v.deadlineBefore(t) {
						res.delete(key)
					}
					i++
				}
				res.mutex.Unlock()
			case <-res.close:
				return
			}
		}
	}()

	return res
}

// Get 获取本地缓存数据
func (l *BuildInMapCache) Get(ctx context.Context, key string) (any, error) {
	l.mutex.RLock()
	v, ok := l.m[key]
	l.mutex.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w, key: %s", errs.ErrKeyNotFound, key)
	}

	now := time.Now()
	// 如果当前已经过期了，进行删除操作
	if v.deadlineBefore(now) {
		l.mutex.Lock()
		defer l.mutex.Unlock()
		// 这里对if规则再进行校验，防止当前锁被Set操作拿到时，数据被进行了过期更新；被Delete操作拿到时，数据被删除掉
		// （双重锁校验:double-check）
		v, ok = l.m[key]
		if !ok {
			return nil, fmt.Errorf("%w, key: %s", errs.ErrKeyNotFound, key)
		}
		if v.deadlineBefore(now) {
			l.delete(key)
			return nil, fmt.Errorf("%w, key: %s", errs.ErrKeyNotFound, key)
		}

	}

	return v.value, nil
}

// Set 设置本地缓存，其中expireTime等于0时，代表永不过期
func (l *BuildInMapCache) Set(ctx context.Context, key string, value any, expireTime time.Duration) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.set(ctx, key, value, expireTime)
}

func (l *BuildInMapCache) set(ctx context.Context, key string, value any, expireTime time.Duration) error {
	i := &item{value: value}
	if expireTime > 0 {
		i.expireTime = time.Now().Add(expireTime)
	}
	l.m[key] = i
	return nil
}

func (l *BuildInMapCache) Delete(ctx context.Context, key string) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.delete(key)
	return nil
}

func (l *BuildInMapCache) LoadAndDelete(ctx context.Context, key string) (any, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	v, ok := l.m[key]
	if !ok {
		return nil, fmt.Errorf("%w, key: %s", errs.ErrKeyNotFound, key)
	}
	l.delete(key)
	return v.value, nil
}

func (l *BuildInMapCache) delete(key string) {
	val, ok := l.m[key]
	if !ok {
		return
	}
	delete(l.m, key)
	l.onEvicted(key, val.value)
}

// Close 关闭本地缓存定期过期校验
func (l *BuildInMapCache) Close() error {
	select {
	case l.close <- struct{}{}:
	default:
		return errors.New("重复关闭")
	}
	return nil
}
