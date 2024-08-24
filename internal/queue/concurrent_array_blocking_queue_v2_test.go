package queue

import (
	"context"
	"github.com/stretchr/testify/require"
	"math/rand"
	"sync"
	"testing"
	"time"
)

// go test -benchmem -bench . -cpu="2,4,8,16" -benchtime=50x .
// goos: windows
// goarch: amd64
// pkg: go_utils/internal/queue
// cpu: AMD Ryzen 9 7940H w/ Radeon 780M Graphics
// BenchmarkConcurrentArrayBlockingQueueV2-2             50           2339658 ns/op         1082363 B/op      18143 allocs/op
// BenchmarkConcurrentArrayBlockingQueueV2-4             50           2123472 ns/op         1121927 B/op      18722 allocs/op
// BenchmarkConcurrentArrayBlockingQueueV2-8             50           2174688 ns/op         1146585 B/op      18989 allocs/op
// BenchmarkConcurrentArrayBlockingQueueV2-16            50           2474864 ns/op         1136189 B/op      18860 allocs/op
// PASS
// ok      go_utils/internal/queue 0.512s
func BenchmarkConcurrentArrayBlockingQueueV2(b *testing.B) {
	q := NewConcurrentArrayBlockingQueueV2[int](10)
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		var wg sync.WaitGroup
		wg.Add(1000)
		for i := 0; i < 1000; i++ {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				val := rand.Int()
				err := q.Enqueue(ctx, val)
				cancel()
				require.NoError(b, err)
			}()
		}
		go func() {
			for i := 0; i < 1000; i++ {
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					_, err := q.Dequeue(ctx)
					cancel()
					require.NoError(b, err)
					wg.Done()
				}()
			}
		}()
		wg.Wait()
	}
}
