# go_utils

go工具类

## list

### ArrayList

基于泛型实现的arrayList, 其中
1. NewArrayList 初始化一个len为0，cap为cap的ArrayList
2. NewArrayListOf 直接使用传入的切片，而不会执行复制

### linkedList

LinkedList 是一个双向循环链表，所以非常适合频繁修改数据的场景, 其中
1. NewLinkedList 初始化一个长度为0的链表
2. NewLinkedListOf 根据传入的切片进行初始化链表

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


## sync

### once

当前官方的包执行的方法中，不支持返回error，这里实现的once支持返回error，且当返回error时，once内的标志位重置

使用方法：

```go
func main() {
   err := once.Do(func() error {
        xxxx // 相关业务逻辑
        return nil
   })
}
```
