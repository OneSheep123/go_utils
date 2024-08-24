package queue

import (
	"context"
	"sync"
)

var _ Queue[any] = &ConcurrentArrayBlockingQueue[any]{}

type ConcurrentArrayBlockingQueue[T any] struct {
	data []T
	head int
	tail int
	size int

	mutex     *sync.RWMutex
	readCond  *cond
	writeCond *cond
}

// NewConcurrentArrayBlockingQueue 新建一个并发阻塞队列
func NewConcurrentArrayBlockingQueue[T any](size int) *ConcurrentArrayBlockingQueue[T] {
	mu := &sync.RWMutex{}
	res := &ConcurrentArrayBlockingQueue[T]{
		data:      make([]T, size),
		mutex:     mu,
		readCond:  newCond(mu),
		writeCond: newCond(mu),
	}
	return res
}

func (c *ConcurrentArrayBlockingQueue[T]) Dequeue(ctx context.Context) (T, error) {
	if ctx.Err() != nil {
		var t T
		return t, ctx.Err()
	}
	c.mutex.Lock()
	// 这里使用for，因为唤醒之后获取到锁这段过程中，队列可能又为空了
	for c.size == 0 {
		ch := c.readCond.singleCh()
		select {
		case <-ctx.Done():
			var t T
			return t, ctx.Err()
		case <-ch:
			c.mutex.Lock()
		}
	}
	res := c.data[c.head]
	var t T
	// 为了释放内存，GC
	c.data[c.head] = t

	c.head++
	c.size--
	if c.head == cap(c.data) {
		c.head = 0
	}
	c.writeCond.broadcast()
	return res, nil
}

func (c *ConcurrentArrayBlockingQueue[T]) Enqueue(ctx context.Context, val T) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	c.mutex.Lock()
	for c.size == len(c.data) {
		// 注意：这里接下来要进行睡眠，因此里面会把锁释放
		ch := c.writeCond.singleCh()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
			c.mutex.Lock()
		}
	}
	c.data[c.tail] = val
	c.tail++
	c.size++

	// c.tail 已经是最后一个了，重置下标
	if c.tail == cap(c.data) {
		c.tail = 0
	}

	c.readCond.broadcast()
	return nil
}

func (c *ConcurrentArrayBlockingQueue[T]) AsSlice() []T {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	res := make([]T, 0, c.size)
	cnt := 0
	capacity := cap(c.data)
	for cnt < c.size {
		index := (c.head + cnt) % capacity
		res = append(res, c.data[index])
		cnt++
	}
	return res
}

func (c *ConcurrentArrayBlockingQueue[T]) Len() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.size
}
