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

// signalCh 返回一个 channel，用于监听广播信号
// 必须在锁范围内使用
// 调用后，锁会被释放，这也是为了确保用户必然是在锁范围内调用的
func (c *cond) signalCh() <-chan struct{} {
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

// Comparator 用于比较两个对象的大小 src < dst, 返回-1，src = dst, 返回0，src > dst, 返回1
// 不要返回任何其它值！
type Comparator[T any] func(src T, dst T) int

// RealNumber 实数
// 绝大多数情况下，你都应该用这个来表达数字的含义
type RealNumber interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~float32 | ~float64
}

type Number interface {
	RealNumber | ~complex64 | ~complex128
}

func ComparatorRealNumber[T RealNumber](src T, dst T) int {
	if src < dst {
		return -1
	} else if src == dst {
		return 0
	} else {
		return 1
	}
}
