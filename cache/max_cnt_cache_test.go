package cache

import (
	"context"
	"github.com/stretchr/testify/assert"
	"go_utils/internal/errs"
	"testing"
	"time"
)

func TestBuildMaxCntCache(t *testing.T) {
	baseCache := NewBuildInMapCache(1 * time.Second)
	testCase := []struct {
		name    string
		key     string
		val     int
		cache   func() *MaxCntCache
		wantErr error
	}{
		{
			name: "The quantity limit is exceeded",
			key:  "key4",
			cache: func() *MaxCntCache {
				cache := NewBuildMaxCntCache(baseCache, 3)
				ctx := context.Background()
				cache.Set(ctx, "key1", 12, 0)
				cache.Set(ctx, "key2", 12, 0)
				cache.Set(ctx, "key3", 12, 0)
				return cache
			},
			wantErr: errs.ErrOverCapacity,
		},
	}

	for _, ts := range testCase {
		maxCntCache := ts.cache()
		err := maxCntCache.Set(context.Background(), ts.key, ts.val, 0)
		assert.Equal(t, ts.wantErr, err)
	}
}
