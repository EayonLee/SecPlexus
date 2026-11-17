package kafkachannel

import (
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/kafka"
)

// 回归：消费侧必须把安全选项接上，否则生产能连、消费连不上。
func TestConsumerOptionsIncludeSecurity(t *testing.T) {
	t.Setenv("SPX_TEST_KAFKA_PASSWORD", "s3cret")

	entry := input.Entry{
		ID: "c1",
		Kafka: input.KafkaBinding{
			Brokers: []string{"broker:9092"},
			Topic:   "logs",
		},
	}

	plain, err := consumerOptions(entry, kafka.Security{})
	if err != nil {
		t.Fatal(err)
	}

	secured, err := consumerOptions(entry, kafka.Security{
		Protocol: "sasl_ssl", Mechanism: "SCRAM-SHA-512",
		Username: "svc", PasswordEnv: "SPX_TEST_KAFKA_PASSWORD",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 明文 5 个基础选项；SASL_SSL 再追加 TLS + SASL 两个。
	if len(plain) != 5 {
		t.Fatalf("明文选项数 = %d，期望 5", len(plain))
	}
	if len(secured) != 7 {
		t.Fatalf("安全选项数 = %d，期望 7", len(secured))
	}
}

// 非法安全配置必须在建立客户端之前失败。
func TestConsumerOptionsRejectBadSecurity(t *testing.T) {
	entry := input.Entry{ID: "c1", Kafka: input.KafkaBinding{Brokers: []string{"broker:9092"}, Topic: "logs"}}

	if _, err := consumerOptions(entry, kafka.Security{Protocol: "mtls"}); err == nil {
		t.Fatal("未知协议被接受")
	}

	if _, err := consumerOptions(entry, kafka.Security{Protocol: "sasl_ssl", Mechanism: "PLAIN"}); err == nil {
		t.Fatal("缺少凭据时没有报错")
	}
}
