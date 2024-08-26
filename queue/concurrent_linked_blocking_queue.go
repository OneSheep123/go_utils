package queue

import (
	"context"
	"go_utils/list"
	"sync"
)

var _ Queue[any] = &ConcurrentLinkedBlockingQueue[any]{}

type ConcurrentLinkedBlockingQueue[T any] struct {
	mu *sync.RWMutex

	maxSize    int
	linkedList *list.LinkedList[T]

	readCond  *cond
	writeCond *cond
}

// NewConcurrentLinkedBlockingQueue 创建链式阻塞队列 capacity <= 0 时，为无界队列
func NewConcurrentLinkedBlockingQueue[T any](capacity int) *ConcurrentLinkedBlockingQueue[T] {
	mutex := &sync.RWMutex{}
	return &ConcurrentLinkedBlockingQueue[T]{
		mu:         mutex,
		maxSize:    capacity,
		readCond:   newCond(mutex),
		writeCond:  newCond(mutex),
		linkedList: list.NewLinkedList[T](),
	}
}

func (c *ConcurrentLinkedBlockingQueue[T]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.linkedList.Len()
}

func (c *ConcurrentLinkedBlockingQueue[T]) AsSlice() []T {
	c.mu.RLock()
	defer c.mu.RUnlock()
	res := c.linkedList.AsSlice()
	return res
}

func (c *ConcurrentLinkedBlockingQueue[T]) Enqueue(ctx context.Context, val T) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	c.mu.Lock()
	for c.maxSize > 0 && c.linkedList.Len() == c.maxSize {
		ch := c.writeCond.signalCh()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
			c.mu.Lock()
		}
	}

	err := c.linkedList.Append(val)
	if err != nil {
		c.mu.Unlock()
		return err
	}
	// 这里会释放锁
	c.readCond.broadcast()
	return err
}

func (c *ConcurrentLinkedBlockingQueue[T]) Dequeue(ctx context.Context) (T, error) {
	if ctx.Err() != nil {
		var t T
		return t, ctx.Err()
	}
	c.mu.Lock()
	for c.linkedList.Len() == 0 {
		signal := c.readCond.signalCh()
		select {
		case <-ctx.Done():
			var t T
			return t, ctx.Err()
		case <-signal:
			c.mu.Lock()
		}
	}

	val, err := c.linkedList.Delete(0)
	c.writeCond.broadcast()
	return val, err
}
