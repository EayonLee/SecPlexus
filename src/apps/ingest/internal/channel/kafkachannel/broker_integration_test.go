//go:build integration

package kafkachannel_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/kafkachannel"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

// 这组用例需要真实 broker：SPX_TEST_KAFKA_BROKERS，例如 10.41.84.81:9092。
// 顺序与位移语义只能在真实总线上验证——内存替身证明不了重投。

func brokers(t *testing.T) []string {
	t.Helper()

	raw := os.Getenv("SPX_TEST_KAFKA_BROKERS")
	if raw == "" {
		t.Skip("SPX_TEST_KAFKA_BROKERS is required")
	}

	parts := strings.Split(raw, ",")
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}

	return parts
}

// 首启从最早开始：接入点启用前积压的消息也要收进来，否则上线即漏数据。
func TestConsumerReadsBacklogFromEarliest(t *testing.T) {
	seed := brokers(t)
	topic := uniqueTopic(t, seed)

	produce(t, seed, topic, []string{`{"src":"10.0.0.1"}`, `{"src":"10.0.0.2"}`, `{"src":"10.0.0.3"}`})

	entry := input.Entry{ID: uniqueGroup(t), Protocol: input.ProtocolKafka, Kafka: input.KafkaBinding{Brokers: seed, Topic: topic}}

	registry := input.NewMemory()
	registry.Replace([]input.Entry{entry})

	line := pipeline.New(pipeline.Options{Workers: 1, BatchSize: 1, FlushInterval: 20 * time.Millisecond}, &nullPublisher{})
	collector := &collectingPublisher{}
	consumer := kafkachannel.New(registry, line, collector, kafkachannel.Options{ReloadInterval: 50 * time.Millisecond})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() { defer close(done); _ = consumer.Start(ctx) }()
	go func() { _ = line.Start(ctx) }()

	waitFor(t, 15*time.Second, func() bool { return collector.count() >= 3 })

	cancel()
	<-done

	if collector.count() < 3 {
		t.Fatalf("消费到 %d 条，期望 3 条（最早位移）", collector.count())
	}
}

// 发布失败绝不提交位移：同一消费组重来一次必须重新收到同一批消息。
// 这是"不丢数据"的唯一可观测证据，内存替身无法验证。
func TestOffsetsStayUncommittedWhenPublishFails(t *testing.T) {
	seed := brokers(t)
	topic := uniqueTopic(t, seed)
	group := uniqueGroup(t)

	produce(t, seed, topic, []string{`{"src":"10.0.0.9"}`})

	// 第一轮：发布始终失败，位移不能推进。
	runConsumer(t, seed, topic, group, &failingPublisher{}, 3*time.Second)

	// 第二轮：同一消费组、可用的发布端，必须重新收到那条消息。
	collector := &collectingPublisher{}
	runConsumerUntil(t, seed, topic, group, collector, 20*time.Second, func() bool { return collector.count() >= 1 })

	if collector.count() < 1 {
		t.Fatal("发布失败的那条消息没有重新投递：位移被错误提交了")
	}
}

// runConsumerUntil 跑到条件满足或超时为止，避免用例空等满一个窗口。
func runConsumerUntil(t *testing.T, seed []string, topic, group string, publisher pipeline.Publisher, timeout time.Duration, done func() bool) {
	t.Helper()

	entry := input.Entry{ID: group, Protocol: input.ProtocolKafka, Kafka: input.KafkaBinding{Brokers: seed, Topic: topic}}

	registry := input.NewMemory()
	registry.Replace([]input.Entry{entry})

	line := pipeline.New(pipeline.Options{Workers: 1, BatchSize: 1, FlushInterval: 20 * time.Millisecond}, publisher)
	consumer := kafkachannel.New(registry, line, publisher, kafkachannel.Options{ReloadInterval: 50 * time.Millisecond})

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	finished := make(chan struct{})
	go func() { defer close(finished); _ = consumer.Start(ctx) }()
	go func() { _ = line.Start(ctx) }()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) && !done() {
		time.Sleep(50 * time.Millisecond)
	}

	cancel()
	<-finished
}

func runConsumer(t *testing.T, seed []string, topic, group string, publisher pipeline.Publisher, duration time.Duration) {
	t.Helper()

	entry := input.Entry{ID: group, Protocol: input.ProtocolKafka, Kafka: input.KafkaBinding{Brokers: seed, Topic: topic}}

	registry := input.NewMemory()
	registry.Replace([]input.Entry{entry})

	line := pipeline.New(pipeline.Options{Workers: 1, BatchSize: 1, FlushInterval: 20 * time.Millisecond}, publisher)
	consumer := kafkachannel.New(registry, line, publisher, kafkachannel.Options{ReloadInterval: 50 * time.Millisecond})

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	done := make(chan struct{})
	go func() { defer close(done); _ = consumer.Start(ctx) }()
	go func() { _ = line.Start(ctx) }()

	<-ctx.Done()
	<-done
}

func produce(t *testing.T, seed []string, topic string, payloads []string) {
	t.Helper()

	client, err := kgo.NewClient(kgo.SeedBrokers(seed...), kgo.DefaultProduceTopic(topic))
	if err != nil {
		t.Fatalf("连接 broker 失败: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	records := make([]*kgo.Record, 0, len(payloads))
	for _, payload := range payloads {
		records = append(records, &kgo.Record{Topic: topic, Value: []byte(payload)})
	}

	if err = client.ProduceSync(ctx, records...).FirstErr(); err != nil {
		t.Fatalf("写入测试消息失败: %v", err)
	}
}

func uniqueTopic(t *testing.T, seed []string) string {
	t.Helper()

	name := fmt.Sprintf("spx-it-%d", time.Now().UnixNano())

	client, err := kgo.NewClient(kgo.SeedBrokers(seed...))
	if err != nil {
		t.Fatalf("连接 broker 失败: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 用低层请求建主题：franz-go 的便捷方法在 kgo 之外，这里只依赖 kmsg。
	request := kmsg.NewPtrCreateTopicsRequest()
	request.TimeoutMillis = 10000
	topic := kmsg.NewCreateTopicsRequestTopic()
	topic.Topic = name
	topic.NumPartitions = 1
	topic.ReplicationFactor = 1
	request.Topics = append(request.Topics, topic)

	response, err := request.RequestWith(ctx, client)
	if err != nil {
		t.Fatalf("创建测试主题失败: %v", err)
	}

	for _, result := range response.Topics {
		if result.ErrorCode == 0 {
			continue
		}

		// 主题已存在视为成功：并发跑用例时可能撞名。
		message := ""
		if result.ErrorMessage != nil {
			message = *result.ErrorMessage
		}
		if !strings.Contains(message, "already exists") {
			t.Fatalf("创建测试主题失败: %s", message)
		}
	}

	return name
}

func uniqueGroup(t *testing.T) string {
	t.Helper()

	return fmt.Sprintf("it-%d", time.Now().UnixNano())
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}

		time.Sleep(50 * time.Millisecond)
	}
}

type collectingPublisher struct {
	mu   sync.Mutex
	seen int
}

func (p *collectingPublisher) Publish(_ context.Context, events []pipeline.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.seen += len(events)

	return nil
}

func (p *collectingPublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.seen
}

type failingPublisher struct{}

func (*failingPublisher) Publish(context.Context, []pipeline.Event) error {
	return errors.New("broker unreachable")
}

type nullPublisher struct{}

func (*nullPublisher) Publish(context.Context, []pipeline.Event) error { return nil }
