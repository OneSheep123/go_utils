package queue

import (
	"context"
	"go_utils/queue"
	"sync"
)

var _ queue.Queue[any] = &ConcurrentArrayQueue[any]{}

// ConcurrentArrayQueue 测试使用，这个队列不提供超时控制，只做学习使用
type ConcurrentArrayQueue[T any] struct {
	data []T
	head int
	tail int
	size int

	l         *sync.RWMutex
	readCond  *sync.Cond
	writeCond *sync.Cond
}

func (c *ConcurrentArrayQueue[T]) Enqueue(ctx context.Context, val T) error {
	c.l.Lock()
	defer c.l.Unlock()
	for c.size == len(c.data) {
		c.writeCond.Wait()
	}
	c.data[c.tail] = val
	c.tail++
	c.size++

	// c.tail 已经是最后一个了，重置下标
	if c.tail == cap(c.data) {
		c.tail = 0
	}
	c.readCond.Signal()
	return nil
}

func (c *ConcurrentArrayQueue[T]) Dequeue(ctx context.Context) (T, error) {
	c.l.Lock()
	defer c.l.Unlock()
	for c.size == 0 {
		c.readCond.Wait()
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
	c.writeCond.Signal()
	return res, nil
}

func NewConcurrentArrayQueue[T any](size int) *ConcurrentArrayQueue[T] {
	mu := &sync.RWMutex{}
	return &ConcurrentArrayQueue[T]{
		data:      make([]T, size),
		l:         mu,
		readCond:  sync.NewCond(mu),
		writeCond: sync.NewCond(mu),
	}
}
