package input

import (
	"context"
	"errors"
)

// ErrKafkaPosition 表示分区必须核对位移或主题身份后显式恢复。
var ErrKafkaPosition = errors.New("Kafka partition position is unavailable")

// KafkaWindow 是本次拉取中同一分区的记录范围及 broker 确认位置。
type KafkaWindow struct {
	Partition int32
	First     int64
	Last      int64
	Committed int64 // 下一条已确认位移；-1 表示未找到消费组位移。
}

// KafkaBoundary 是分区从指定位置起使用的不可变配置。
type KafkaBoundary struct {
	Offset int64
	Source Entry
}

// KafkaExecutionStore 在发布前固定 Kafka 批次的配置，即使确认未知也可复用。
type KafkaExecutionStore interface {
	Bind(context.Context, Entry, string, KafkaWindow) ([]KafkaBoundary, error)
}
