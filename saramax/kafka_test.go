package saramax

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/IBM/sarama"
	"github.com/stretchr/testify/require"
	"io"
	"log"
	"math/rand"
	"strconv"
	"testing"
	"time"
)

var (
	syncTopic = "cch_test"
)

type KafkaData struct {
	OrderId string `json:"order_id"`
}

func TestKafkaProducer(t *testing.T) {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Return.Successes = true
	config.Producer.Return.Errors = true
	config.Version, _ = sarama.ParseKafkaVersion("0.10.2.1")
	producer, err := sarama.NewSyncProducer([]string{"172.16.3.14:9092"}, config)
	if err != nil {
		log.Fatalln(err)
		return
	}
	for i := 0; i < 300; i++ {
		rand.Seed(time.Now().UnixNano())
		randomNum := rand.Intn(300) + 1
		log.Printf("生成的随机数: %d", randomNum)
		data := KafkaData{OrderId: fmt.Sprintf("%d", randomNum)}
		bytes, _ := json.Marshal(data)
		message := &sarama.ProducerMessage{
			Topic:     syncTopic,
			Value:     sarama.ByteEncoder(bytes),
			Partition: int32(randomNum) % 3,
		}
		partition, offset, err := producer.SendMessage(message)
		if err != nil {
			log.Fatalln(err)
			return
		}
		log.Printf("当前偏移量为:%d, 分区为: %d\n", offset, partition)
	}
	safeClose(t, producer)
}

func safeClose(t testing.TB, c io.Closer) {
	t.Helper()
	err := c.Close()
	if err != nil {
		t.Error(err)
	}
}

func initConsumerNoAutoCommit(t *testing.T) *sarama.ConsumerGroup {
	config := sarama.NewConfig()
	config.Consumer.Return.Errors = true
	config.Version, _ = sarama.ParseKafkaVersion("0.10.2.1")
	config.Consumer.Offsets.Initial = sarama.OffsetNewest
	config.Consumer.Offsets.AutoCommit.Enable = false
	consumerGroup, err := sarama.NewConsumerGroup([]string{"172.16.3.14:9092"}, "cch_test_consumer_no_auto_commit", config)
	require.NoError(t, err)
	return &consumerGroup
}

func initConsumerAutoCommit(t *testing.T) *sarama.ConsumerGroup {
	config := sarama.NewConfig()
	config.Consumer.Return.Errors = true
	config.Version, _ = sarama.ParseKafkaVersion("0.10.2.1")
	config.Consumer.Offsets.Initial = sarama.OffsetNewest
	config.Consumer.Offsets.AutoCommit.Enable = true
	consumerGroup, err := sarama.NewConsumerGroup([]string{"172.16.3.14:9092"}, "cch_test_consumer_auto_commit", config)
	require.NoError(t, err)
	return &consumerGroup
}

type TestConsumerHandler struct {
	// 注意里面要进行手动提交
	handleMessageByHand func(session sarama.ConsumerGroupSession, message *sarama.ConsumerMessage) error
}

func (t *TestConsumerHandler) Setup(session sarama.ConsumerGroupSession) error {
	fmt.Printf("消费者分配到新分区: %v\n", (session).Claims())
	return nil
}

func (t *TestConsumerHandler) Cleanup(session sarama.ConsumerGroupSession) error {
	fmt.Printf("分区被回收，提交最终偏移量\n")
	(session).Commit() // 确保在分区被回收前提交
	return nil
}

func (t *TestConsumerHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}

			err := t.handleMessageByHand(session, msg)
			if err != nil {
				fmt.Println("处理消息失败", err)
			}
		case <-session.Context().Done():
			return nil
		}
	}
}

func NewTestConsumerHandler(handleMessageByHand func(session sarama.ConsumerGroupSession, message *sarama.ConsumerMessage) error) *TestConsumerHandler {
	return &TestConsumerHandler{
		handleMessageByHand: handleMessageByHand,
	}
}

func TestKafkaConsumer(t *testing.T) {
	channels, _ := NewKafkaChannel(defaultChannelNum, defaultChannelLen)
	consumerGroup := *initConsumerNoAutoCommit(t)
	handler := NewTestConsumerHandler(func(session sarama.ConsumerGroupSession, msg *sarama.ConsumerMessage) error {
		// 捕获panic防止消费者崩溃
		defer func() {
			if r := recover(); r != nil {
				fmt.Println("panic", r)
			}
		}()
		// 记录消息信息
		fmt.Println("TestKafkaConsumer kafka_msg", "msg_topic", msg.Topic, "msg_partition", msg.Partition, "msg_offset", msg.Offset, "msg_val", string(msg.Value))
		// 并发处理消息，将消息分发到不同的通道以便并行处理。根据订单ID选择通道，为了保证同一订单的消息按顺序处理。
		err := channels.PushKafkaMsgToChannels(session, msg, getChannelIndexByOrderId(msg, defaultChannelNum), func(k kafkaStruct) {
			message := k.consumerMessage.Value
			var kafkaOrderData KafkaData
			err := json.Unmarshal(message, &kafkaOrderData)
			if err != nil {
				fmt.Println(err)
			}
			fmt.Println("data", kafkaOrderData.OrderId)
		})
		if err != nil {
			fmt.Println("TestKafkaConsumer PushKafkaMsgToChannels err", "err", err, "msg_val", string(msg.Value))
		}
		return nil
	})
	for {
		if err := consumerGroup.Consume(context.Background(), []string{syncTopic}, handler); err != nil {
			fmt.Println(err)
			return
		}
	}
}

// 根据订单ID获取通道索引
func getChannelIndexByOrderId(msg *sarama.ConsumerMessage, channelNum int64) int {
	var kafkaOrderData KafkaData
	_ = json.Unmarshal(msg.Value, &kafkaOrderData)
	atoi, _ := strconv.Atoi(kafkaOrderData.OrderId)
	return atoi
}

func TestKafkaConsumerAutoCommit(t *testing.T) {
	consumerGroup := *initConsumerAutoCommit(t)
	handler := NewTestConsumerHandler(func(session sarama.ConsumerGroupSession, msg *sarama.ConsumerMessage) error {
		// 记录消息信息
		fmt.Println("TestKafkaConsumer kafka_msg", "msg_topic", msg.Topic, "msg_partition", msg.Partition, "msg_offset", msg.Offset, "msg_val", string(msg.Value))
		var kafkaOrderData KafkaData
		err := json.Unmarshal(msg.Value, &kafkaOrderData)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println("data", kafkaOrderData.OrderId)
		return nil
	})
	for {
		if err := consumerGroup.Consume(context.Background(), []string{syncTopic}, handler); err != nil {
			fmt.Println(err)
			return
		}
	}
}
