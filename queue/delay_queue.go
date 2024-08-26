package queue

import (
	"context"
	"sync"
	"time"
)

type Delayable interface {
	// Deadline 是还剩下多少过期时间
	// 还要延迟多久
	Deadline() time.Time
}

type DelayQueue[T Delayable] struct {
	queue     *PriorityQueue[T]
	lock      *sync.Mutex
	readCond  *cond
	writeCond *cond
	zero      T
}

// NewDelayQueue 新建一个延时队列，当size<=0时，表示无界
func NewDelayQueue[T Delayable](size int) *DelayQueue[T] {
	queue := NewPriorityQueue[T](size, func(src T, dst T) int {
		srcDeadline := src.Deadline()
		dstDeadline := dst.Deadline()
		// peak的时候，队首为最小的（小顶堆）
		if srcDeadline.Before(dstDeadline) {
			return -1
		} else if srcDeadline.After(dstDeadline) {
			return 1
		}
		return 0
	})
	lock := &sync.Mutex{}
	return &DelayQueue[T]{
		queue:     queue,
		lock:      lock,
		readCond:  newCond(lock),
		writeCond: newCond(lock),
	}
}

// Enqueue 入队操作
func (d *DelayQueue[T]) Enqueue(ctx context.Context, val T) error {
	d.lock.Lock()
	for {
		if ctx.Err() != nil {
			d.lock.Unlock()
			return ctx.Err()
		}
		err := d.queue.Enqueue(ctx, val)
		switch err {
		case nil:
			// 唤醒所有的读等待，开始进行接收
			// 注意：里面会有释放锁的操作
			d.readCond.broadcast()
			return err
		case ErrOutOfCapacity:
			// 注意：里面会进行解锁操作，主要这里是防止自己既在阻塞，又拿着锁不释放
			ch := d.writeCond.signalCh()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ch:
				d.lock.Lock()
			}
		default:
			// 进行解锁操作，不应该占用着锁
			d.lock.Unlock()
			return err
		}
	}
}

// Dequeue 出队操作
// 1. 先检测队列有没有元素，没有要阻塞，直到超时，或者拿到元素
// 2. 有元素，你是不是要看一眼，队头的元素的过期时间有没有到
// 2.1 如果过期时间到了，直接出队并且返回
// 2.2 如果过期时间没到，阻塞直到过期时间到了
// 2.2.1 如果在等待的时候，有新元素到了，就要看一眼新元素的过期时间是不是更短
// 2.2.2 如果等待的时候，ctx 超时了，那么就直接返回超时错误
func (d *DelayQueue[T]) Dequeue(ctx context.Context) (T, error) {
	for {
		select {
		case <-ctx.Done():
			return d.zero, ctx.Err()
		default:
		}
		d.lock.Lock()
		if d.queue.isEmpty() {
			ch := d.readCond.signalCh()
			select {
			case <-ch:
			case <-ctx.Done():
				return d.zero, ctx.Err()
			}
		} else {
			data, _ := d.queue.Peek()
			now := time.Now()
			// 当前元素已经到达过期时间
			if now.After(data.Deadline()) {
				data, err := d.queue.Dequeue(ctx)
				d.writeCond.broadcast()
				return data, err
			}
			// 此时当前元素还未到达超时时间
			ticker := time.NewTicker(data.Deadline().Sub(now))
			// 调用signalCh
			// 1. 这里要进行释放锁，以免下面占用着锁，又在超时等待
			// 2. 需要获取channel进行阻塞，以便有新元素来了可以被进行唤醒通知
			ch := d.readCond.signalCh()
			select {
			case <-ctx.Done():
				return d.zero, ctx.Err()
			case <-ticker.C:
			case <-ch:
				// 说明此时有新元素进来了
			}
		}
	}
}
