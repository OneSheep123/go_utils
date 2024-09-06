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

### 基于链表的实现 ConcurrentLinkedBlockingQueue

ConcurrentLinkedBlockingQueue 是基于链表的实现，它分成有界和无界两种形态。如果在创建队列的时候传入的容量小于等于0，那么就代表这是一个无界的并发阻塞队列。在无界的情况下，入队永远不会阻塞。但是队列为空的时候，出队依旧会阻塞。

使用方法非常简单：

```go
func ExampleNewConcurrentLinkedBlockingQueue() {
    // 创建一个容量为 10 的有界并发阻塞队列，如果传入 0 或者负数，那么创建的是无界并发阻塞队列
    q := NewConcurrentLinkedBlockingQueue[int](10)
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

### 延时队列 DelayQueue

DelayQueue，即延时队列。延时队列的运作机制是：

- 按照元素的预期过期时间来进行排序，过期时间短的在前面；
- 当从队列中获取元素的时候，如果队列为空，或者元素还没有到期，那么调用者会被阻塞；直到超时
- 入队的时候，如果队列已经满了，那么调用者会被阻塞，直到有人取走元素，或者阻塞超时；
因此延时队列的使用场景主要就是那些依赖于时间的场景，例如本地缓存，定时任务等。

使用延时队列需要两个步骤：

- 实现 Delayable 接口
- 创建一个延时队列

使用举例
```go
func ExampleNewDelayQueue() {
	q := NewDelayQueue[delayElem](10)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	now := time.Now()
	_ = q.Enqueue(ctx, delayElem{
		// 3 秒后过期
		deadline: now.Add(time.Second * 3),
		val:      3,
	})

	_ = q.Enqueue(ctx, delayElem{
		// 2 秒后过期
		deadline: now.Add(time.Second * 2),
		val:      2,
	})

	_ = q.Enqueue(ctx, delayElem{
		// 1 秒后过期
		deadline: now.Add(time.Second * 1),
		val:      1,
	})

	var vals []int
	val, _ := q.Dequeue(ctx)
	vals = append(vals, val.val)
	val, _ = q.Dequeue(ctx)
	vals = append(vals, val.val)
	val, _ = q.Dequeue(ctx)
	vals = append(vals, val.val)
	fmt.Println(vals)
	duration := time.Since(now)
	if duration > time.Second*3 {
		fmt.Println("delay!")
	}
	// Output:
	// [1 2 3]
	// delay!
}
```

## cache

### localCache

本地缓存实现，其中对于过期键使用惰性删除和过期删除

使用方法:
```go
func main() {
   // 新建一个本地缓存，其中配置定期2秒进行过期检测
   localCache := NewBuildInMapCache(2*time.Second, func(cache *BuildInMapCache) {
        cache.onEvicted = func(key string, value any) {
        
        }
   })
   // 写入一个key，其中过期时间为1ms
   localCache.Set(context.Background(), "key1", 12, time.Millisecond)
   // 写入一个key，其中不过期
   localCache.Set(context.Background(), "key2", 12, 0)
}
```

### maxCntCache
基于本地缓存封装的控制键值对数量缓存

### lruCache
基于LRU算法实现的本地缓存

### redisLock
基于redis的分布式锁实现

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

## ginx

gin工具库

### middleware

#### ratelimit

请求限流中间件，当前有实现基于redis的限流

使用方法: 

```go
func main() {
   server := gin.Default()
   // 创建一个基于 redis 的滑动窗口限流器(1秒只允许有10个请求)
   lm := ratelimit.NewRedisSlidingWindowLimiter(redisClient, 1 * time.Second, 10)
   svc := ratelimit.NewBuilder(lm).SetKeyGenFunc(func(*gin.Context) string {
	   // 设置滑动窗口中限制的ip
	   return ctx.ClientIp()
   })
   server.Use(svc.Build())
}
```
