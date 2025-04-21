package saramax

import (
	"github.com/IBM/sarama"
	"testing"
	"time"
)

func TestKafkaConsumerV2_1(t *testing.T) {
	config := sarama.NewConfig()
	config.Consumer.Return.Errors = true
	config.Version, _ = sarama.ParseKafkaVersion("0.10.2.1")
	config.Consumer.Offsets.Initial = sarama.OffsetOldest
	config.Consumer.Offsets.AutoCommit.Enable = false
	kafkaAddress := "172.16.3.14:9092"
	consumerGroup := "cch_test_v2"
	topic := "cch_test"

	testKafkaConsumer(t, config, kafkaAddress, consumerGroup, topic)

}
func TestKafkaConsumerV2_2(t *testing.T) {
	config := sarama.NewConfig()
	config.Consumer.Return.Errors = true
	config.Version, _ = sarama.ParseKafkaVersion("0.10.2.1")
	config.Consumer.Offsets.Initial = sarama.OffsetOldest
	config.Consumer.Offsets.AutoCommit.Enable = false
	kafkaAddress := "172.16.3.14:9092"
	consumerGroup := "cch_test_v2"
	topic := "cch_test"

	testKafkaConsumer(t, config, kafkaAddress, consumerGroup, topic)

}

func testKafkaConsumer(t *testing.T, config *sarama.Config, kafkaAddress string, consumerGroup string, topic string) {
	consumer := NewKafkaConsumer([]string{kafkaAddress}, []string{topic}, consumerGroup, 100, config)
	consumer.StartConsume(func(msg *sarama.ConsumerMessage) error {
		time.Sleep(10 * time.Second)
		t.Log(string(msg.Value))
		return nil
	})
}

// # 重制位移脚本
//kafka-consumer-groups.sh --bootstrap-server 172.16.3.14:9092 \
//--group cch_test_v2 \
//--reset-offsets --to-earliest \
//--topic cch_test \
//--execute
