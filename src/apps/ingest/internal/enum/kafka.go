package enum

// KafkaStartPosition 是消费组没有已提交位移时的起始位置。
type KafkaStartPosition string

const (
	KafkaStartEarliest     KafkaStartPosition = "earliest"
	KafkaStartSinceCreated KafkaStartPosition = "since_created"
)

// Valid 检查对外允许的起始位置。
func (p KafkaStartPosition) Valid() bool {
	return p == KafkaStartEarliest || p == KafkaStartSinceCreated
}
