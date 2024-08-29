// create by chencanhua in 2023/9/12
package cache

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go_utils/internal/errs"
	"testing"
	"time"
)

func TestBuildInMapCache_Get(t *testing.T) {
	testCase := []struct {
		name    string
		key     string
		cache   func() *BuildInMapCache
		wantVal any
		wantErr error
	}{
		{
			name: "key not found",
			key:  "not exist key",
			cache: func() *BuildInMapCache {
				return NewBuildInMapCache(10 * time.Second)
			},
			wantErr: fmt.Errorf("%w, key: %s", errs.ErrKeyNotFound, "not exist key"),
		},
		{
			name: "get value",
			key:  "key1",
			cache: func() *BuildInMapCache {
				res := NewBuildInMapCache(10 * time.Second)
				err := res.Set(context.Background(), "key1", 123, time.Minute)
				require.NoError(t, err)
				return res
			},
			wantVal: 123,
		},
		{
			name: "expire value",
			key:  "key2",
			cache: func() *BuildInMapCache {
				res := NewBuildInMapCache(10 * time.Second)
				err := res.Set(context.Background(), "key2", 123, time.Second)
				require.NoError(t, err)
				time.Sleep(3 * time.Second)
				return res
			},
			wantErr: fmt.Errorf("%w, key: %s", errs.ErrKeyNotFound, "not exist key"),
		},
	}

	for _, ts := range testCase {
		localCache := ts.cache()
		val, err := localCache.Get(context.Background(), ts.key)
		assert.Equal(t, ts.wantErr, err)
		if err != nil {
			return
		}
		assert.Equal(t, ts.wantVal, val)
	}
}

func TestBuildInMapCache_Loop(t *testing.T) {
	count := 0
	localCache := NewBuildInMapCache(2*time.Second, func(cache *BuildInMapCache) {
		cache.onEvicted = func(key string, value any) {
			count++
		}
	})
	err := localCache.Set(context.Background(), "key1", 12, time.Millisecond)
	require.NoError(t, err)
	time.Sleep(3 * time.Second)
	// 这里没有去调用Get方法，以免是Get操作导致key被删除
	localCache.mutex.RLock()
	defer localCache.mutex.RUnlock()
	_, ok := localCache.m["key1"]
	require.Equal(t, false, ok)
}
