package saramax

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBM/sarama"
)

const (
	defaultChannelNum         = 100
	defaultChannelLen         = 1000
	defaultBatchSize          = 200
	InactiveChannelTimeOut    = 60 * time.Second
	ChannelHandleMsgCheckTime = 10 * time.Second
	KafkaCommitCheckTime      = 10 * time.Second
)

// kafkaStruct 封装Kafka消费组会话和消息
type kafkaStruct struct {
	consumerGroupSession sarama.ConsumerGroupSession
	consumerMessage      *sarama.ConsumerMessage
}

// ChannelWrapper 带状态的通道包装器
type ChannelWrapper struct {
	ch             chan kafkaStruct // 消息通道
	chState        *ChannelState    // 通道状态
	batchProcessor *BatchProcessor  // 批处理器
}

// PartitionCoordinator 分区协调器，管理同一分区的多个处理通道
type PartitionCoordinator struct {
	topic         string                      // 主题
	partition     int32                       // 分区
	session       sarama.ConsumerGroupSession // 消费组会话
	channels      []*ChannelWrapper           // 分区下的所有处理通道
	commitMu      sync.Mutex                  // 提交操作互斥锁
	watermark     int64                       // 当前可提交的水位线偏移
	lastCommitted int64                       // 最后提交的偏移量
	triggerChan   chan struct{}               // 提交触发通道
	stopChan      chan struct{}               // 停止信号通道
	wg            sync.WaitGroup              // 等待协程退出
}

// ChannelState 单个通道的处理状态
type ChannelState struct {
	processedOffset int64      // 已处理的最大偏移量
	pendingCount    int32      // 待处理消息计数（原子操作）
	mu              sync.Mutex // 状态操作互斥锁
	isActive        bool       // 通道活跃状态标记
	lastActiveTime  time.Time  // 最后活跃时间
}

func (c *ChannelState) activate() {
	c.mu.Lock()
	if !c.isActive {
		c.isActive = true
		c.lastActiveTime = time.Now()
	}
	c.mu.Unlock()
}

func (c *ChannelState) updateProcessedOffset(offset int64) {
	// 更新处理偏移量
	c.mu.Lock()
	c.processedOffset = offset
	c.lastActiveTime = time.Now()
	c.isActive = true
	c.mu.Unlock()
}

// KafkaChannelStruct Kafka通道管理结构
type KafkaChannelStruct struct {
	channelNum             int                                        // 每个分区的通道数量
	topicPartitionChannels map[string]map[int32]*PartitionCoordinator // 主题-分区-协调器映射
	channelLen             int                                        // 单个通道缓冲长度
	mu                     sync.RWMutex                               // 结构操作读写锁
}

// BatchProcessor 批处理器
type BatchProcessor struct {
	batchSize        int           // 批次大小
	partitionBatches []kafkaStruct // 当前批次消息
	mu               sync.Mutex    // 批次操作互斥锁
}

func (b *BatchProcessor) Add(msg kafkaStruct) {
	b.mu.Lock()
	b.partitionBatches = append(b.partitionBatches, msg)
	b.mu.Unlock()
}

func (b *BatchProcessor) ready() bool {
	return len(b.partitionBatches) >= b.batchSize
}

func (b *BatchProcessor) handleMsg(handleFunc func(kafkaStruct)) {
	for _, msg := range b.partitionBatches {
		handleFunc(msg)
	}
}

func (b *BatchProcessor) getMaxOffset() int64 {
	return b.partitionBatches[len(b.partitionBatches)-1].consumerMessage.Offset
}

func (b *BatchProcessor) resetPartitionBatches() {
	b.mu.Lock()
	b.partitionBatches = b.partitionBatches[:0]
	b.mu.Unlock()
}

func (b *BatchProcessor) processAndReset(s *ChannelState, handleFunc func(kafkaStruct)) {
	// 使用CAS确保单次处理
	if !atomic.CompareAndSwapInt32(&s.pendingCount, 0, 1) {
		return
	}
	defer atomic.StoreInt32(&s.pendingCount, 0)
	b.handleMsg(handleFunc)
	s.updateProcessedOffset(b.getMaxOffset())
	b.resetPartitionBatches()
}

// NewBatchProcessor 创建批处理器
func NewBatchProcessor(batchSize int) *BatchProcessor {
	return &BatchProcessor{
		batchSize:        batchSize,
		partitionBatches: make([]kafkaStruct, 0, batchSize), // 预分配内存
	}
}

func NewChannelState() *ChannelState {
	return &ChannelState{
		processedOffset: -1, // -1表示初始状态
		isActive:        true,
		lastActiveTime:  time.Now(),
	}
}

func NewChannelWrapper(channelLen int) *ChannelWrapper {
	return &ChannelWrapper{
		ch:             make(chan kafkaStruct, channelLen),
		chState:        NewChannelState(),
		batchProcessor: NewBatchProcessor(defaultBatchSize),
	}
}

// NewKafkaChannel 创建Kafka通道管理器
func NewKafkaChannel(channelNum, channelLen int) (*KafkaChannelStruct, error) {
	if channelNum <= 0 {
		return nil, errors.New("channelNum must be positive")
	}
	if channelLen < 0 {
		return nil, errors.New("channelLen must be non-negative")
	}
	return &KafkaChannelStruct{
		channelNum:             channelNum,
		channelLen:             channelLen,
		topicPartitionChannels: make(map[string]map[int32]*PartitionCoordinator),
	}, nil
}

// NewPartitionCoordinator 创建分区协调器
func NewPartitionCoordinator(topic string, partition int32, session sarama.ConsumerGroupSession, channelNum, channelLen int) *PartitionCoordinator {
	// 初始化分区所有通道
	channels := make([]*ChannelWrapper, channelNum)
	for i := 0; i < channelNum; i++ {
		channels[i] = NewChannelWrapper(channelLen)
	}
	// 创建分区协调器
	coordinator := &PartitionCoordinator{
		topic:       topic,
		partition:   partition,
		session:     session,
		channels:    channels,
		triggerChan: make(chan struct{}, 1), // 缓冲1避免阻塞
		stopChan:    make(chan struct{}),
	}
	coordinator.wg.Add(1)
	go coordinator.monitor()
	return coordinator
}

// CloseAllChannel 关闭所有通道
func (k *KafkaChannelStruct) CloseAllChannel() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	for _, partitions := range k.topicPartitionChannels {
		for _, coordinator := range partitions {
			// 先关闭所有消息通道
			for _, ch := range coordinator.channels {
				close(ch.ch)
			}
			//再停止协调器
			coordinator.Stop()
		}
	}
	return nil
}

// PushKafkaMsgToChannels 推送消息到指定通道（优化版）
func (k *KafkaChannelStruct) PushKafkaMsgToChannels(session sarama.ConsumerGroupSession, msg *sarama.ConsumerMessage, index int, handleFunc func(kafkaStruct)) error {
	// 路由消息到指定通道
	if index < 0 || index >= k.channelNum {
		return fmt.Errorf("invalid channel index: %d (range: 0-%d)", index, k.channelNum-1)
	}
	topic := msg.Topic
	partition := msg.Partition
	// 双检查锁模式优化初始化
	k.mu.RLock()
	coordinator, exists := k.topicPartitionChannels[topic][partition]
	k.mu.RUnlock()
	if !exists {
		k.mu.Lock()
		// 二次检查防止重复创建
		if _, ok := k.topicPartitionChannels[topic]; !ok {
			k.topicPartitionChannels[topic] = make(map[int32]*PartitionCoordinator)
		}
		if _, ok := k.topicPartitionChannels[topic][partition]; !ok {
			coordinator = NewPartitionCoordinator(topic, partition, session, k.channelNum, k.channelLen)
			k.topicPartitionChannels[topic][partition] = coordinator
			// 异步启动处理协程
			go func() {
				for _, wrapper := range coordinator.channels {
					k.HandleKafkaMsgWithBatch(topic, partition, wrapper, handleFunc)
				}
			}()
		}
		k.mu.Unlock()
	}
	// 获取目标通道（此时coordinator必定非nil）
	wrapper := coordinator.channels[index]
	select {
	case wrapper.ch <- kafkaStruct{consumerGroupSession: session, consumerMessage: msg}:
		wrapper.chState.activate()
		return nil
	}
}

// HandleKafkaMsgWithBatch 批量消息处理方法
func (k *KafkaChannelStruct) HandleKafkaMsgWithBatch(topic string, partition int32, channel *ChannelWrapper, handleFunc func(kafkaStruct)) {
	go func(wrapper *ChannelWrapper) {
		processor := wrapper.batchProcessor
		partitionCoordinator := k.topicPartitionChannels[topic][partition]
		state := wrapper.chState
		ticker := time.NewTicker(ChannelHandleMsgCheckTime)
		defer ticker.Stop()
		for {
			select {
			case data, ok := <-wrapper.ch:
				if !ok {
					processBatchIfNeeded(processor, state, handleFunc, partitionCoordinator, false)
					return
				}
				processMessage(data, processor, state, partitionCoordinator, handleFunc)
			case <-ticker.C:
				processBatchIfNeeded(processor, state, handleFunc, partitionCoordinator, true)
			}
		}
	}(channel)
}

// processMessage 处理单个消息
func processMessage(data kafkaStruct, bp *BatchProcessor, cs *ChannelState, pc *PartitionCoordinator, handleFunc func(kafkaStruct)) {
	bp.Add(data)
	if bp.ready() {
		bp.processAndReset(cs, handleFunc)
		pc.triggerCommit()
	}
}

func (pc *PartitionCoordinator) checkCommitMinOffset() (finalMinOffset int64, totalPending int32) {
	activeMinOffset, hasActiveChannel, now := int64(math.MaxInt64), false, time.Now()
	//第一轮获取活跃通道最小偏移量 & 统计所有待处理消息
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for _, channel := range pc.channels {
			channel.chState.mu.Lock()
			// 检查超时：60秒无更新视为非活跃
			if now.Sub(channel.chState.lastActiveTime) > InactiveChannelTimeOut {
				channel.chState.isActive = false
			}
			if channel.chState.isActive && channel.chState.processedOffset < activeMinOffset {
				activeMinOffset = channel.chState.processedOffset
				hasActiveChannel = true
			}
			channel.chState.mu.Unlock()
		}
	}()
	go func() {
		defer wg.Done()
		for _, channel := range pc.channels {
			totalPending += atomic.LoadInt32(&channel.chState.pendingCount)
		}
	}()
	wg.Wait()
	// 确定最终提交偏移量
	finalMinOffset = pc.watermark
	if hasActiveChannel {
		finalMinOffset = activeMinOffset
	}
	// 提交条件检查
	return finalMinOffset, totalPending
}

// tryCommit 尝试提交偏移量（分区协调器方法）
func (pc *PartitionCoordinator) tryCommit() {
	finalMinOffset, totalPending := pc.checkCommitMinOffset()
	//提交条件检查
	if finalMinOffset > pc.watermark && totalPending == 0 {
		go func() {
			pc.session.MarkOffset(pc.topic, pc.partition, finalMinOffset+1, "")
			fmt.Println("tryCommit commit offset", "topic", pc.topic, "partition", pc.partition, "offset", finalMinOffset+1)
			pc.session.Commit()
		}()
		pc.commitMu.Lock()
		pc.lastCommitted = finalMinOffset
		pc.watermark = finalMinOffset
		pc.commitMu.Unlock()
	}
}

// 定时处理检查
func processBatchIfNeeded(bp *BatchProcessor, cs *ChannelState, handleFunc func(kafkaStruct), pc *PartitionCoordinator, checkPending bool) {
	if len(bp.partitionBatches) > 0 && (!checkPending || atomic.LoadInt32(&cs.pendingCount) == 0) {
		bp.processAndReset(cs, handleFunc)
		pc.triggerCommit()
	}
}

// 触发提交检查
func (pc *PartitionCoordinator) triggerCommit() {
	select {
	case pc.triggerChan <- struct{}{}:
	default: // 避免阻塞
	}
}

func (pc *PartitionCoordinator) monitor() {
	defer pc.wg.Done()
	ticker := time.NewTicker(KafkaCommitCheckTime)
	defer ticker.Stop()
	for {
		select {
		case <-pc.triggerChan:
			pc.tryCommit()
		case <-ticker.C:
			pc.tryCommit()
		case <-pc.stopChan:
			pc.tryCommit()
			return
		}
	}
}

//func (pc *PartitionCoordinator) finalCommit() {
//	var maxOffset int64 = -1
//	for _, channel := range pc.channels {
//		state := channel.chState
//		state.mu.Lock()
//		if state.processedOffset > maxOffset {
//			maxOffset = state.processedOffset
//		}
//		state.mu.Unlock()
//	}
//	if maxOffset > pc.lastCommitted {
//		pc.session.MarkOffset(pc.topic, pc.partition, maxOffset+1, "")
//		pc.session.Commit()
//	}
//}

func (pc *PartitionCoordinator) Stop() {
	close(pc.stopChan)
	pc.wg.Wait()
}
