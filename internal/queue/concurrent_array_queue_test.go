package queue

import (
	"context"
	"github.com/stretchr/testify/require"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func TestConcurrentArrayQueue(t *testing.T) {
	// 并发测试，只是测试有没有死锁之类的问题
	// 先进先出这个特性依赖于其它单元测试
	// 也依赖于代码审查
	q := NewConcurrentArrayQueue[int](100)
	var wg sync.WaitGroup
	wg.Add(1000)
	for i := 0; i < 1000; i++ {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			val := rand.Int()
			err := q.Enqueue(ctx, val)
			cancel()
			require.NoError(t, err)
		}()
	}
	go func() {
		for i := 0; i < 1000; i++ {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				_, err := q.Dequeue(ctx)
				cancel()
				require.NoError(t, err)
				wg.Done()
			}()
		}
	}()
	wg.Wait()
}
