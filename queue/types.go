package queue

import (
	"context"
	"sync"
)

type Queue[T any] interface {
	Enqueue(ctx context.Context, val T) error
	Dequeue(ctx context.Context) (T, error)
}

type cond struct {
	single chan struct{}
	l      sync.Locker
}

func newCond(l sync.Locker) *cond {
	return &cond{
		l:      l,
		single: make(chan struct{}),
	}
}

// singleCh 返回一个 channel，用于监听广播信号
// 必须在锁范围内使用
// 调用后，锁会被释放，这也是为了确保用户必然是在锁范围内调用的
func (c *cond) singleCh() <-chan struct{} {
	res := c.single
	c.l.Unlock()
	return res
}

// broadcast 唤醒等待者
// 如果没有人等待，那么什么也不会发生
// 必须加锁之后才能调用这个方法
// 广播之后锁会被释放，这也是为了确保用户必然是在锁范围内调用的
func (c *cond) broadcast() {
	single := make(chan struct{})
	old := c.single
	c.single = single
	c.l.Unlock()
	close(old)
}
