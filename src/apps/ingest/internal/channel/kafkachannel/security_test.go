package kafkachannel

import (
	"context"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connection/kafka"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
)

// 回归：消费侧必须把安全选项接上，否则生产能连、消费连不上。
func TestConsumerOptionsIncludeSecurity(t *testing.T) {

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
		Username: "svc", Password: "test-password",
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		opts []kgo.Opt
		tls  bool
	}{
		{plain, false}, {secured, true},
	} {
		client, err := kgo.NewClient(test.opts...)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()

		if client.OptValue(kgo.DialTLS) != test.tls {
			t.Fatalf("TLS 配置不匹配，期望 %v", test.tls)
		}
		mechanisms := client.OptValue(kgo.SASL).([]sasl.Mechanism)
		if !test.tls && len(mechanisms) != 0 {
			t.Fatal("明文连接不应配置 SASL")
		}
		if test.tls && (len(mechanisms) != 1 || mechanisms[0].Name() != "SCRAM-SHA-512") {
			t.Fatal("安全连接应使用 SCRAM-SHA-512")
		}
	}
}

// 非法安全配置必须在建立客户端之前失败。
func TestConsumerOptionsRejectBadSecurity(t *testing.T) {
	entry := input.Entry{ID: "c1", Kafka: input.KafkaBinding{Brokers: []string{"broker:9092"}, Topic: "logs", Credential: "unsupported"}}

	consumer := New(nil, nil, nil, Options{Credentials: map[string]kafka.SourceCredential{"unsupported": {Brokers: []string{"broker:9092"}, Security: kafka.Security{Protocol: "mtls"}}}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := consumer.consume(ctx, entry, worker{}); err == nil {
		t.Fatal("未知协议被接受")
	}

	if _, err := consumerOptions(entry, kafka.Security{Protocol: "sasl_ssl", Mechanism: "PLAIN"}); err == nil {
		t.Fatal("缺少凭据时没有报错")
	}
}

func TestSourceCredentialsAreBoundToConfiguredBrokers(t *testing.T) {
	profile := kafka.SourceCredential{Brokers: []string{"edge.local:9093", "edge-2.local:9093"}}
	if !profile.Allows([]string{"edge.local:9093"}) ||
		profile.Allows([]string{"edge.local:9093", "unregistered.local:9093"}) ||
		profile.Allows(nil) {
		t.Fatal("credential lookup did not enforce the configured broker list")
	}
}
