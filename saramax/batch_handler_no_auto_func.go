package saramax

import (
	"context"
	"crypto/sha1"
	"fmt"
	"github.com/IBM/sarama"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
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
	k.Processor.SetCommitStep(100)
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
func (k *KafkaConsumer) SetCommitStep(step int) {
	k.Processor.SetCommitStep(step)
}

func (k *KafkaConsumer) SetCommitIn(commitInterval time.Duration) {
	k.Processor.SetCommitIn(commitInterval)
}

// SetConsumeFromBeginningForTopic 设置从头开始消费
// 设置从头开始消费
func (k *KafkaConsumer) SetConsumeFromBeginningForTopic(topic string) {
	// 创建offset manager
	client, err := sarama.NewClient(k.Brokers, k.Config)
	if err != nil {
		panic(fmt.Sprintf("Error creating client: %v", err))
	}
	defer func(client sarama.Client) {
		err := client.Close()
		if err != nil {
			fmt.Printf("Error closing client: %v\n", err)
		}
	}(client)
	// 创建offset manager
	offsetManager, err := sarama.NewOffsetManagerFromClient(k.ConsumerGroup, client)
	if err != nil {
		panic(fmt.Sprintf("Error creating offset manager: %v", err))
	}
	defer func(offsetManager sarama.OffsetManager) {
		err := offsetManager.Close()
		if err != nil {
			fmt.Printf("Error closing offset manager: %v\n", err)
		}
	}(offsetManager)

	partitions, _ := client.Partitions(topic)
	for _, partition := range partitions {
		partitionManager, _ := offsetManager.ManagePartition(topic, partition)
		partitionManager.ResetOffset(0, "") // 重置到起始位置
		err := partitionManager.Close()
		if err != nil {
			fmt.Printf("Error closing partition manager: %v\n", err)
		}
	}
}

func (k *KafkaConsumer) SetConsumeFromBeginning() {
	// 创建offset manager
	client, err := sarama.NewClient(k.Brokers, k.Config)
	if err != nil {
		panic(fmt.Sprintf("Error creating client: %v", err))
	}
	defer func(client sarama.Client) {
		err := client.Close()
		if err != nil {
			fmt.Printf("Error closing client: %v\n", err)
		}
	}(client)
	// 创建offset manager
	offsetManager, err := sarama.NewOffsetManagerFromClient(k.ConsumerGroup, client)
	if err != nil {
		panic(fmt.Sprintf("Error creating offset manager: %v", err))
	}
	defer func(offsetManager sarama.OffsetManager) {
		err := offsetManager.Close()
		if err != nil {
			fmt.Printf("Error closing offset manager: %v\n", err)
		}
	}(offsetManager)

	for _, topic := range k.Topics {
		partitions, _ := client.Partitions(topic)
		for _, partition := range partitions {
			partitionManager, _ := offsetManager.ManagePartition(topic, partition)
			partitionManager.ResetOffset(0, "") // 重置到起始位置
			err := partitionManager.Close()
			if err != nil {
				fmt.Printf("Error closing partition manager: %v\n", err)
			}
		}
	}

}

// StartConsume 启动消费者，调用时传入处理函数
func (k *KafkaConsumer) StartConsume(process func(msg *sarama.ConsumerMessage) error) {

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
	k.close()
}

// close 关闭消费者
func (k *KafkaConsumer) close() {
	k.Processor.Close()
}

// KafkaProcessor 实现处理逻辑
type KafkaProcessor struct {
	ready       chan struct{}
	msgChannels []chan *sarama.ConsumerMessage
	offsetChan  chan *partitionOffset
	closeOnce   sync.Once
	sessionOnce sync.Once
	session     sarama.ConsumerGroupSession
	hashFunc    func(msg *sarama.ConsumerMessage) uint32
	// 提交步长
	CommitStep int
	// 提交间隔
	CommitInterval time.Duration
}

type partitionOffset struct {
	Topic     string
	Partition int32
	Offset    int64
	Status    bool
}

func NewKafkaProcessor(workerChannels int) *KafkaProcessor {
	p := &KafkaProcessor{
		ready:          make(chan struct{}),
		msgChannels:    make([]chan *sarama.ConsumerMessage, workerChannels),
		offsetChan:     make(chan *partitionOffset, workerChannels*2),
		CommitStep:     100,
		CommitInterval: 3 * time.Second,
	}

	// 初始化工作通道
	for i := range p.msgChannels {
		p.msgChannels[i] = make(chan *sarama.ConsumerMessage, workerChannelLength)
	}

	return p
}

func (p *KafkaProcessor) SetCommitStep(step int) {
	p.CommitStep = step
}

func (p *KafkaProcessor) SetCommitIn(commitInterval time.Duration) {
	p.CommitInterval = commitInterval
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

func (p *KafkaProcessor) StartWorker(process func(msg *sarama.ConsumerMessage) error) {
	// 启动工作协程
	for i := range p.msgChannels {
		go p.worker(i, process)
	}
	// 启动偏移量提交协程
	go func() {
		err := p.commitWorker()
		if err != nil {
			fmt.Printf("Commit worker error: %v\n", err)
		}
	}()
}

// ConsumeClaim
// 消费消息
// 1. 消费消息
// 2. 哈希分发到工作通道
func (p *KafkaProcessor) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	p.sessionOnce.Do(func() {
		p.session = session
	})
	for msg := range claim.Messages() {
		//msgOffset := msg.Offset
		p.offsetChan <- &partitionOffset{
			Topic:     msg.Topic,
			Partition: msg.Partition,
			Offset:    msg.Offset,
			Status:    false,
		}
		// 哈希分发逻辑
		// 如果没有自定义哈希函数，则使用默认的哈希函数
		hashFunc := p.getHashValue
		if p.hashFunc != nil {
			hashFunc = p.hashFunc
		}
		hashValue := hashFunc(msg)

		chIndex := hashValue % uint32(len(p.msgChannels))

		fmt.Printf("worker %d consume msg topic %s partition %d offset %d\n", chIndex, msg.Topic, msg.Partition, msg.Offset)
		// 将消息发送到对应的工作通道
		p.msgChannels[chIndex] <- msg
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
func (p *KafkaProcessor) worker(chIndex int, process func(msg *sarama.ConsumerMessage) error) {
	for msg := range p.msgChannels[chIndex] {
		// 业务处理逻辑
		if err := process(msg); err == nil {
			// 处理成功后提交偏移量
			p.offsetChan <- &partitionOffset{
				Topic:     msg.Topic,
				Partition: msg.Partition,
				Offset:    msg.Offset,
				Status:    true,
			}
		}
	}
}

// 提交偏移量的工作协程
func (p *KafkaProcessor) commitWorker() error {
	// 记录每个分区当前可提交的最大偏移量
	partitionMap := make(map[string]*partitionOffset)
	// 记录上一次已提交的偏移量
	lastPartitionMap := make(map[string]*partitionOffset)
	// 记录每个分区中已成功处理的偏移量(true表示已处理)
	partitionOffsets := make(map[string]map[int64]bool)
	// 记录每个分区待处理的偏移量队列
	offsets := make(map[string][]int64)

	ticker := time.NewTicker(p.CommitInterval) // 兜底提交间隔
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			// 定时提交
			for partitionStr, pOffset := range partitionMap {
				fmt.Printf("定时执行 topic %s offset %d,lastoffset %d \n", partitionStr, pOffset.Offset, lastPartitionMap[partitionStr].Offset)
				if pOffset.Offset > lastPartitionMap[partitionStr].Offset {
					p.commit(pOffset)
					lastPartitionMap[partitionStr].Offset = pOffset.Offset
					fmt.Printf("定时提交 topic %s partition %d offset %d\n", pOffset.Topic, pOffset.Partition, pOffset.Offset)
				}
			}
			break
		case parOffset, ok := <-p.offsetChan:
			if !ok {
				// 通道关闭
				return nil
			}
			currentPartition := parOffset.Partition
			currentOffset := parOffset.Offset
			currentTopic := parOffset.Topic
			currentStatus := parOffset.Status
			currentPartitionStr := fmt.Sprintf("%s-%d", currentTopic, currentPartition)
			if _, exists := partitionMap[currentPartitionStr]; !exists {
				partitionMap[currentPartitionStr] = &partitionOffset{Topic: currentTopic, Partition: currentPartition, Offset: currentOffset - 1}
			}
			if _, exists := lastPartitionMap[currentPartitionStr]; !exists {
				lastPartitionMap[currentPartitionStr] = &partitionOffset{Topic: currentTopic, Partition: currentPartition, Offset: currentOffset - 1}
			}
			if _, exists := partitionOffsets[currentPartitionStr]; !exists {
				partitionOffsets[currentPartitionStr] = make(map[int64]bool)
			}
			if _, exists := offsets[currentPartitionStr]; !exists {
				offsets[currentPartitionStr] = make([]int64, 0)
			}

			if currentStatus == false {
				offsets[currentPartitionStr] = append(offsets[currentPartitionStr], currentOffset)
				continue
			}
			partitionOffsets[currentPartitionStr][currentOffset] = true
			// 检查待处理队列，找出连续成功处理的最大偏移量
			maxOffsetIndex := -1
			for index, offset := range offsets[currentPartitionStr] {
				if _, exists := partitionOffsets[currentPartitionStr][offset]; exists {
					partitionMap[currentPartitionStr].Offset = offset
					maxOffsetIndex = index
					// 释放已提交的偏移量
					delete(partitionOffsets[currentPartitionStr], offset)
				} else {
					break
				}
			}
			if maxOffsetIndex > -1 {
				// 删除offsets在maxOffsetIndex之前的元素
				offsets[currentPartitionStr] = offsets[currentPartitionStr][maxOffsetIndex+1:]
				// 提交偏移量
				if partitionMap[currentPartitionStr].Offset >= lastPartitionMap[currentPartitionStr].Offset+int64(p.CommitStep) {
					p.commit(partitionMap[currentPartitionStr])
					lastPartitionMap[currentPartitionStr].Offset = partitionMap[currentPartitionStr].Offset
				}
			}
		}
	}
}

// 提交处理
func (p *KafkaProcessor) commit(tracker *partitionOffset) {
	// 按分区维护最大偏移量
	fmt.Printf("提交 topic %s partition %d offset %d\n", tracker.Topic, tracker.Partition, tracker.Offset)
	// 实际提交代码示例：
	p.session.MarkOffset(tracker.Topic, tracker.Partition, tracker.Offset+1, "")
	p.session.Commit()
}
