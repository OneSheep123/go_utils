package cache

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

func TestNewLRUCache(t *testing.T) {
	lruCache := NewBuildLRUCache(2)
	wg := sync.WaitGroup{}
	wg.Add(20)
	for i := 0; i < 10; i++ {
		go func(i int) {
			lruCache.Set(context.Background(), fmt.Sprintf("key+%d", 1), 1, 0)
			wg.Done()
		}(i)
	}
	for i := 0; i < 10; i++ {
		go func(i int) {
			lruCache.Get(context.Background(), fmt.Sprintf("key+%d", 1))
			wg.Done()
		}(i)
	}
	wg.Wait()
}
