# go_utils

go工具类

## queue

并发阻塞队列意味着队列是并发安全的，并且是阻塞式的。所有实现都支持在以下情况阻塞：

- 在入队的时候，如果已经到达容量上限，那么就会阻塞。
- 在出队的时候，如果队列已经为空，那么就会阻塞。

不管出队还是入队，如果此时你传入的 context.Context 参数是可以被取消，或者设置了超时，那么在 context.Context 被取消或者超时的时候会返回错误。你可以通过检测返回的 error 是不是 context.Cancel 或者 context.DeadlineExceeded 来判断是不是被人取消，或者超时了

### 基于切片的实现 ConcurrentArrayBlockingQueue

ConcurrentArrayBlockingQueue 是有界并发阻塞队列。

使用方法非常简单：

```go
func ExampleNewConcurrentArrayBlockingQueue() {
	q := NewConcurrentArrayBlockingQueue[int](10)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = q.Enqueue(ctx, 22)
	val, err := q.Dequeue(ctx)
	// 这是例子，实际中你不需要写得那么复杂
	switch err {
	case context.Canceled:
		// 有人主动取消了，即调用了 cancel 方法。在这个例子里不会出现这个情况
	case context.DeadlineExceeded:
		// 超时了
	case nil:
		fmt.Println(val)
	default:
		// 其它乱七八糟的
	}
	// Output:
	// 22
}

```

另外internal/queue中ConcurrentArrayBlockingQueueV2是另外一种实现，使用semaphore包
