package saramax

import (
	"context"
	"crypto/sha1"
	"fmt"
	"github.com/IBM/sarama"
	"golang.org/x/sync/errgroup"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

const (
	workerChannelLength = 100 // 哈希分发的通道长度
)

type KafkaConsumer struct {
	// Kafka brokers
	Brokers []string
	// Kafka topic
	Topics []string
	// Kafka consumer group
	ConsumerGroup string
	// worker channels
	WorkerChannels int
	// Kafka config
	Config *sarama.Config
	// Kafka processor
	Processor *KafkaProcessor
}

func NewKafkaConsumer(brokers []string, topics []string, consumerGroup string, workerChannels int, config *sarama.Config) *KafkaConsumer {
	k := &KafkaConsumer{
		Brokers:        brokers,
		Topics:         topics,
		ConsumerGroup:  consumerGroup,
		WorkerChannels: workerChannels,
	}

	// 初始化处理器
	k.Processor = NewKafkaProcessor(k.WorkerChannels)

	// 配置消费者
	if config == nil {
		config := sarama.NewConfig()
		config.Version = sarama.V2_8_0_0
		config.Consumer.Offsets.Initial = sarama.OffsetOldest
		config.Consumer.Offsets.AutoCommit.Enable = false
	}
	k.Config = config
	return k
}

// StartConsume 启动消费者，调用时传入处理函数
func (k *KafkaConsumer) StartConsume(process func(msg *Msg) error) {

	// 创建消费者组
	consumer, err := sarama.NewConsumerGroup(k.Brokers, k.ConsumerGroup, k.Config)
	// 启动消费者（此时会从新offset开始消费）
	if err != nil {
		panic(fmt.Sprintf("Error creating consumer: %v", err))
	}
	defer func(consumer sarama.ConsumerGroup) {
		err := consumer.Close()
		if err != nil {
			fmt.Printf("Error closing consumer: %v\n", err)
		}
	}(consumer)

	// 启动工作协程
	k.Processor.StartWorker(process)

	// 启动消费协程
	ctx, cancel := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		for {
			if err := consumer.Consume(ctx, k.Topics, k.Processor); err != nil {
				fmt.Printf("Consume error: %v\n", err)
			}
			if ctx.Err() != nil {
				return
			}
		}
	}()

	// 等待处理器就绪
	<-k.Processor.Ready()
	fmt.Println("consumer is ready")

	// 优雅关闭
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	fmt.Println("\nShutting down...")

	cancel()
	wg.Wait()
}

// close 关闭消费者
func (k *KafkaConsumer) close() {
	k.Processor.Close()
}

type commitedOffsets struct {
	m map[string]*partitionOffset
	sync.RWMutex
}

func (c *commitedOffsets) getPartitionOffset(partitionStr string) *partitionOffset {
	c.RLock()
	defer c.RUnlock()
	return c.m[partitionStr]
}

func (c *commitedOffsets) setPartitionOffset(partitionStr string, p *partitionOffset) {
	c.Lock()
	c.m[partitionStr] = p
	c.Unlock()
}

type Msg struct {
	msg *sarama.ConsumerMessage
	// 注意：每个分区对应的session是不一样的
	session *sarama.ConsumerGroupSession
}

// KafkaProcessor 实现处理逻辑
type KafkaProcessor struct {
	ready                   chan struct{}
	msgChannels             []chan *Msg
	offsetChan              chan *partitionOffset
	closeOnce               sync.Once
	committedOffsets        commitedOffsets // 维护每个分区最后提交的位移
	hashFunc                func(msg *sarama.ConsumerMessage) uint32
	commitGoroutineLimitNum int // 每个管道提交偏移量限制协程数量
}

func (p *KafkaProcessor) getPartitionOffsetStr(topic string, partition int32) string {
	return fmt.Sprintf("%s-%d", topic, partition)
}

type partitionOffset struct {
	Topic     string
	Partition int32
	Offset    int64
	session   *sarama.ConsumerGroupSession
}

func NewKafkaProcessor(workerChannels int) *KafkaProcessor {
	p := &KafkaProcessor{
		ready:                   make(chan struct{}),
		msgChannels:             make([]chan *Msg, workerChannels),
		offsetChan:              make(chan *partitionOffset, workerChannels*2),
		committedOffsets:        commitedOffsets{m: make(map[string]*partitionOffset)},
		commitGoroutineLimitNum: 3,
	}

	// 初始化工作通道
	for i := range p.msgChannels {
		p.msgChannels[i] = make(chan *Msg, workerChannelLength)
	}

	// 启动偏移量提交协程
	go func() {
		fmt.Println("start commit worker")
		err := p.commitWorker()
		if err != nil {
			fmt.Printf("Commit worker error: %v\n", err)
		}
	}()

	return p
}

func (p *KafkaProcessor) Ready() <-chan struct{} {
	return p.ready
}

func (p *KafkaProcessor) Close() {
	p.closeOnce.Do(func() {
		close(p.offsetChan)
		for _, ch := range p.msgChannels {
			close(ch)
		}
	})
}

// Setup 处理器的初始化
func (p *KafkaProcessor) Setup(sarama.ConsumerGroupSession) error {
	close(p.ready)
	return nil
}

func (p *KafkaProcessor) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

func (p *KafkaProcessor) StartWorker(process func(msg *Msg) error) {
	// 启动工作协程
	for i := range p.msgChannels {
		fmt.Println("start msg worker: ", i)
		go p.worker(i, process)
	}
}

// ConsumeClaim
// 消费消息
// 1. 消费消息
// 2. 哈希分发到工作通道
func (p *KafkaProcessor) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		msgPartition := msg.Partition
		msgOffset := msg.Offset
		msgTopic := msg.Topic
		msgPartitionStr := p.getPartitionOffsetStr(msgTopic, msgPartition)
		if v := p.committedOffsets.getPartitionOffset(msgPartitionStr); v == nil {
			p.committedOffsets.setPartitionOffset(msgPartitionStr, &partitionOffset{
				Topic:     msgTopic,
				Partition: msgPartition,
				Offset:    msgOffset - 1,
				session:   &session,
			})
		}
		// 哈希分发逻辑
		// 如果没有自定义哈希函数，则使用默认的哈希函数
		hashFunc := p.getHashValue
		if p.hashFunc != nil {
			hashFunc = p.hashFunc
		}
		hashValue := hashFunc(msg)

		chIndex := hashValue % uint32(len(p.msgChannels))
		// 将消息发送到对应的工作通道
		fmt.Printf("offset: %d 分发消息到通道:%d \n", msgOffset, chIndex)
		p.msgChannels[chIndex] <- &Msg{
			msg:     msg,
			session: &session,
		}
	}
	return nil
}

func (p *KafkaProcessor) SetHashFunc(hashFunc func(msg *sarama.ConsumerMessage) uint32) {
	p.hashFunc = hashFunc
}

// 获取哈希键
func (p *KafkaProcessor) getHashValue(msg *sarama.ConsumerMessage) uint32 {
	// 解析消息获取哈希字段

	// 计算哈希值
	h := sha1.New()
	h.Write([]byte(fmt.Sprintf("%v", msg.Offset)))
	hash := h.Sum(nil)

	return uint32(hash[0])<<24 | uint32(hash[1])<<16 | uint32(hash[2])<<8 | uint32(hash[3])
}

// 工作协程处理业务逻辑
func (p *KafkaProcessor) worker(chIndex int, process func(msg *Msg) error) {
	eg := errgroup.Group{}
	eg.SetLimit(p.commitGoroutineLimitNum)
	for msg := range p.msgChannels[chIndex] {
		// 业务处理逻辑
		if err := process(msg); err == nil {
			// 处理成功后提交偏移量
			fmt.Println("发送偏移量:", msg.msg.Partition, msg.msg.Offset)
			eg.Go(func() error {
				p.offsetChan <- &partitionOffset{
					Topic:     msg.msg.Topic,
					Partition: msg.msg.Partition,
					Offset:    msg.msg.Offset,
					session:   msg.session,
				}
				return nil
			})
		}
	}
}

// 提交偏移量的工作协程
func (p *KafkaProcessor) commitWorker() error {
	// 缓存未按顺序到达的位移
	pendingOffsets := make(map[string][]int64)
	for {
		select {
		case parOffset, ok := <-p.offsetChan:
			if !ok {
				return nil
			}
			fmt.Println("获取偏移量:", parOffset)
			currentPartition := parOffset.Partition
			currentOffset := parOffset.Offset
			currentTopic := parOffset.Topic
			currentPartitionStr := p.getPartitionOffsetStr(currentTopic, currentPartition)
			var lastCommitted *partitionOffset
			var session *sarama.ConsumerGroupSession
			// 1. 获取该分区最后提交的位移
			if v := p.committedOffsets.getPartitionOffset(currentPartitionStr); v == nil {
				//异常报错
				fmt.Println("分区不存在:", currentPartition)
				return nil
			} else {
				lastCommitted = v
				session = v.session
			}
			// 2. 如果是期望的下一个位移
			if currentOffset == lastCommitted.Offset+1 {
				currentPartitionOffset := &partitionOffset{Topic: currentTopic, Partition: currentPartition, Offset: currentOffset, session: session}
				// 提交位移
				p.commit(currentPartitionOffset)
				// 更新最后提交的位移
				p.committedOffsets.setPartitionOffset(currentPartitionStr, currentPartitionOffset)
				// 3. 检查是否有缓存的后续位移可以提交
				for {
					nextOffset := p.committedOffsets.getPartitionOffset(currentPartitionStr).Offset + 1
					if pending, ok := pendingOffsets[currentPartitionStr]; ok {
						found := false
						for i, pendingOffset := range pending {
							if pendingOffset == nextOffset {
								// 提交这个缓存的位移
								currentPartitionOffset = &partitionOffset{Topic: currentTopic, Partition: currentPartition, Offset: pendingOffset, session: session}
								p.commit(currentPartitionOffset)
								// 更新最后提交的位移
								p.committedOffsets.setPartitionOffset(currentPartitionStr, currentPartitionOffset)
								// 从缓存中删除
								pendingOffsets[currentPartitionStr] = append(pending[:i], pending[i+1:]...)
								found = true
								break
							}
						}
						if !found {
							break
						}
					} else {
						break
					}
				}
			} else {
				// 4. 不是期望的下一个位移，先缓存起来
				if currentOffset > lastCommitted.Offset+1 {
					if _, ok = pendingOffsets[currentPartitionStr]; !ok {
						pendingOffsets[currentPartitionStr] = make([]int64, 0)
					}
					pendingOffsets[currentPartitionStr] = append(pendingOffsets[currentPartitionStr], currentOffset)
					fmt.Printf("缓存位移 topic:%s, partition:%d offset:%d (等待:%d)\n",
						currentTopic, currentPartition, currentOffset, lastCommitted.Offset+1)
				}
				// 小于等于lastCommitted的位移直接忽略（已经提交过了）
			}
		}
	}
}

// 提交处理
func (p *KafkaProcessor) commit(tracker *partitionOffset) {
	// 按分区维护最大偏移量
	fmt.Printf("------------------提交 topic %s partition %d offset %d\n", tracker.Topic, tracker.Partition, tracker.Offset+1)
	// 实际提交代码示例：
	(*tracker.session).MarkOffset(tracker.Topic, tracker.Partition, tracker.Offset+1, "")
	(*tracker.session).Commit()
}
