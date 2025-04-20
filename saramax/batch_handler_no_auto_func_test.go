package saramax

import (
	"github.com/IBM/sarama"
	"testing"
)

func TestKafkaConsumerV2(t *testing.T) {
	config := sarama.NewConfig()
	config.Consumer.Return.Errors = true
	config.Version, _ = sarama.ParseKafkaVersion("0.10.2.1")
	config.Consumer.Offsets.Initial = sarama.OffsetOldest
	config.Consumer.Offsets.AutoCommit.Enable = false

	topic := "cch_test"
	// 初始化消费者
	consumer := NewKafkaConsumer([]string{"172.16.3.14:9092"}, []string{topic}, "cch_test_v2", 100, config)
	consumer.StartConsume(func(msg *Msg) error {
		t.Log(string(msg.msg.Value))
		return nil
	})
}

// # 重制位移脚本
//kafka-consumer-groups.sh --bootstrap-server 172.16.3.14:9092 \
//--group cch_test_v2 \
//--reset-offsets --to-earliest \
//--topic cch_test \
//--execute
