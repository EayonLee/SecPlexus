//go:build integration

package kafkachannel_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/kafkachannel"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/publish"
	"github.com/EayonLee/SecPlexus/tests/integration/kafkafixture"
)

// 这组用例需要真实 broker：SPX_TEST_KAFKA_BROKERS。
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
	topic := uniqueTopic(t, seed, 1)

	produce(t, seed, topic, []string{`{"kind":"net","src":"10.0.0.1"}`, `{"kind":"net","src":"10.0.0.2"}`, `{"kind":"net","src":"10.0.0.3"}`})
	rule, err := normalizer.CompileDefinition([]byte(`{
		"contract_version":"v1","parse":{"mode":"json"},
		"branches":[
			{"id":"traffic","name":"Traffic","class_uid":4001,"match":[{"field":"kind","equals":"net"}],"field_map":{"src":"src_endpoint.ip"}},
			{"id":"auth","name":"Authentication","class_uid":3002,"match":[{"field":"kind","equals":"login"}],"field_map":{"username":"user.name","dst":"dst_endpoint.ip"}}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}

	entry := input.Entry{ID: uniqueGroup(t, seed), Mapping: rule, Protocol: input.ProtocolKafka, Kafka: input.KafkaBinding{Brokers: seed, Topic: topic}}

	registry := input.NewMemory()
	registry.Replace([]input.Entry{entry})

	line := pipeline.New(pipeline.Options{Workers: 1, BatchSize: 1, FlushInterval: 20 * time.Millisecond}, &nullPublisher{})
	collector := &collectingPublisher{}
	consumer := kafkachannel.New(registry, line, collector, kafkachannel.Options{ReloadInterval: 50 * time.Millisecond})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)

	done := make(chan struct{})
	go func() { defer close(done); _ = consumer.Start(ctx) }()
	go func() { _ = line.Start(ctx) }()
	defer func() {
		cancel()
		<-done
	}()

	waitFor(t, 15*time.Second, func() bool { return collector.count() >= 3 })

	entry.Vendor, entry.Product = "New vendor", "Live edge"
	registry.Replace([]input.Entry{entry})
	produce(t, seed, topic, []string{`{"kind":"login","username":"example","dst":"10.0.0.4"}`})
	waitFor(t, 5*time.Second, func() bool { return collector.count() >= 4 })

	cancel()
	<-done

	var body struct {
		Class    int `json:"class_uid"`
		Metadata struct {
			SPX struct{ Vendor, Product string }
		}
	}
	if err := json.Unmarshal(collector.lastBody, &body); err != nil {
		t.Fatal(err)
	}
	if body.Class != 3002 || body.Metadata.SPX.Vendor != "New vendor" || body.Metadata.SPX.Product != "Live edge" {
		t.Fatalf("已有连接未采用新的来源配置: %+v", body.Metadata.SPX)
	}
}

// 发布失败绝不提交位移：同一消费组重来一次必须重新收到同一批消息。
// 这是"不丢数据"的唯一可观测证据，内存替身无法验证。
func TestOffsetsStayUncommittedWhenPublishFails(t *testing.T) {
	seed := brokers(t)
	topic := uniqueTopic(t, seed, 1)
	group := uniqueGroup(t, seed)

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

func TestExpiredCommittedOffsetStopsThePartition(t *testing.T) {
	seed := brokers(t)
	topic := uniqueTopic(t, seed, 1)
	group := uniqueGroup(t, seed)
	produce(t, seed, topic, []string{`{"id":"committed"}`})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := kgo.NewClient(kgo.SeedBrokers(seed...))
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	offset := kmsg.NewPtrOffsetFetchRequest()
	offset.Group = "spx-ingest-" + group
	offset.Topics = []kmsg.OffsetFetchRequestTopic{{Topic: topic, Partitions: []int32{0}}}
	initial := &collectingPublisher{}
	runConsumerUntil(t, seed, topic, group, initial, 15*time.Second, func() bool {
		if initial.count() != 1 {
			return false
		}
		response, err := offset.RequestWith(ctx, admin)
		if err != nil {
			return false
		}
		if len(response.Groups) == 1 && len(response.Groups[0].Topics) == 1 && len(response.Groups[0].Topics[0].Partitions) == 1 {
			return response.Groups[0].Topics[0].Partitions[0].Offset == 1
		}
		return len(response.Topics) == 1 && len(response.Topics[0].Partitions) == 1 && response.Topics[0].Partitions[0].Offset == 1
	})
	produce(t, seed, topic, []string{`{"id":"expired"}`, `{"id":"available"}`})
	deleteRecords := kmsg.NewPtrDeleteRecordsRequest()
	deleteRecords.Topics = []kmsg.DeleteRecordsRequestTopic{{Topic: topic, Partitions: []kmsg.DeleteRecordsRequestTopicPartition{{Partition: 0, Offset: 2}}}}
	response, err := deleteRecords.RequestWith(ctx, admin)
	if err != nil || len(response.Topics) != 1 || len(response.Topics[0].Partitions) != 1 || response.Topics[0].Partitions[0].LowWatermark != 2 {
		t.Fatalf("could not advance test log start: %+v, %v", response, err)
	}
	entry := input.Entry{ID: group, Protocol: input.ProtocolKafka, Kafka: input.KafkaBinding{Brokers: seed, Topic: topic}}
	registry := input.NewMemory()
	registry.Replace([]input.Entry{entry})
	collector := &collectingPublisher{}
	line := pipeline.New(pipeline.Options{Workers: 1}, collector)
	consumer := kafkachannel.New(registry, line, collector, kafkachannel.Options{ReloadInterval: 50 * time.Millisecond})
	stopped := make(chan struct{})
	go func() { defer close(stopped); _ = consumer.Start(ctx) }()
	defer func() { cancel(); <-stopped }()

	waitFor(t, 10*time.Second, func() bool {
		state := consumer.Activation(entry)
		return !state.Online && state.Problem == input.KafkaPositionUnavailable
	})
	state := consumer.Activation(entry)
	if state.Online || state.Problem != input.KafkaPositionUnavailable || consumer.OffsetsRejected() == 0 || collector.count() != 0 {
		t.Fatalf("expired offset was not reported and stopped: ready=%t reason=%q rejected=%d published=%d",
			state.Online, state.Problem, consumer.OffsetsRejected(), collector.count())
	}
}

func TestKafkaActivationReportsDrainingUntilOldBatchFinishes(t *testing.T) {
	seed := brokers(t)
	topic := uniqueTopic(t, seed, 1)
	group := uniqueGroup(t, seed)
	produce(t, seed, topic, []string{`{"id":"old"}`})
	old := input.Entry{ID: group, Revision: 1, Protocol: input.ProtocolKafka, Kafka: input.KafkaBinding{Brokers: seed, Topic: topic}}
	current := old
	current.Revision++
	registry := input.NewMemory()
	registry.Replace([]input.Entry{old})
	entered, release := make(chan struct{}, 1), make(chan struct{})
	publisher := publishFunc(func(ctx context.Context, _ []pipeline.Event) error {
		entered <- struct{}{}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	line := pipeline.New(pipeline.Options{Workers: 1}, publisher)
	consumer := kafkachannel.New(registry, line, publisher, kafkachannel.Options{ReloadInterval: 50 * time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	finished := make(chan struct{})
	go func() { defer close(finished); _ = consumer.Start(ctx) }()
	defer func() { cancel(); <-finished }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("old batch did not enter publishing")
	}
	registry.Replace([]input.Entry{current})
	if state := consumer.Activation(current); !state.Draining || state.Online {
		t.Fatalf("old batch was not reported as draining: %+v", state)
	}
	close(release)
	waitFor(t, 5*time.Second, func() bool { return consumer.Activation(current).Online })
	if state := consumer.Activation(current); !state.Online || state.Draining {
		t.Fatalf("new configuration was not confirmed after commit: %+v", state)
	}
}

type publishFunc func(context.Context, []pipeline.Event) error

func (publish publishFunc) Publish(ctx context.Context, events []pipeline.Event) error {
	return publish(ctx, events)
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

// TestPublisherKeepsEntityRecordsInPartition 验证实际 broker 分区和生产批次内的顺序。
func TestPublisherKeepsEntityRecordsInPartition(t *testing.T) {
	seed := brokers(t)
	topic := uniqueTopic(t, seed, 3)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	producer, err := publish.NewKafka(ctx, publish.Options{Brokers: seed, EventTopic: topic})
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()

	const entity = "192.0.2.12"
	events := []pipeline.Event{
		{PartitionKey: entity, Body: []byte("1")},
		{PartitionKey: "198.51.100.8", Body: []byte("other")},
		{PartitionKey: entity, Body: []byte("2")},
		{PartitionKey: entity, Body: []byte("3")},
	}
	if err = producer.Publish(ctx, events); err != nil {
		t.Fatal(err)
	}
	reader, err := kgo.NewClient(kgo.SeedBrokers(seed...), kgo.ConsumeTopics(topic))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	var ordered []string
	partition := int32(-1)
	for len(ordered) < 3 {
		results := reader.PollFetches(ctx)
		if err = results.Err(); err != nil {
			t.Fatal(err)
		}
		results.EachRecord(func(record *kgo.Record) {
			if string(record.Key) != entity {
				return
			}
			if partition >= 0 && record.Partition != partition {
				t.Fatalf("same entity written to partitions %d and %d", partition, record.Partition)
			}
			partition = record.Partition
			ordered = append(ordered, string(record.Value))
		})
	}
	if !slices.Equal(ordered, []string{"1", "2", "3"}) {
		t.Fatalf("unexpected Kafka order: %v", ordered)
	}
}

func TestPublisherReturnsOnlyUnconfirmedRecordsInBatch(t *testing.T) {
	seed := brokers(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	topic := uniqueTopic(t, seed, 1)
	producer, err := publish.NewKafka(ctx, publish.Options{
		Brokers: seed, EventTopic: topic,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()

	pending, err := pipeline.PublishPending(ctx, producer, []pipeline.Event{
		{EventID: "confirmed-first", Body: []byte("first")},
		{EventID: "too-large", Body: make([]byte, (4<<20)+1)},
		{EventID: "confirmed-last", Body: []byte("last")},
	})
	if err == nil || len(pending) != 1 || pending[0].EventID != "too-large" {
		t.Fatalf("unexpected Kafka partial result: %d pending, %v", len(pending), err)
	}
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

func uniqueTopic(t *testing.T, seed []string, partitions int32) string {
	t.Helper()

	name := "spx-it-" + uuid.NewString()
	client, err := kgo.NewClient(kgo.SeedBrokers(seed...))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	if err := kafkafixture.CreateTopic(client, name, partitions); err != nil {
		t.Fatalf("创建测试主题 %s: %v", name, err)
	}
	t.Cleanup(func() {
		if err := kafkafixture.DeleteTopic(client, name); err != nil {
			t.Errorf("回收测试主题 %s: %v", name, err)
		}
	})
	return name
}

func uniqueGroup(t *testing.T, seed []string) string {
	t.Helper()
	id := "it-" + uuid.NewString()
	t.Cleanup(func() {
		client, err := kgo.NewClient(kgo.SeedBrokers(seed...))
		if err != nil {
			t.Error(err)
			return
		}
		defer client.Close()
		if err := kafkafixture.DeleteGroup(client, "spx-ingest-"+id); err != nil {
			t.Errorf("回收测试消费组 %s: %v", id, err)
		}
	})
	return id
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
	mu       sync.Mutex
	seen     int
	lastBody []byte
}

func (p *collectingPublisher) Publish(_ context.Context, events []pipeline.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.seen += len(events)
	if len(events) > 0 {
		p.lastBody = events[len(events)-1].Body
	}

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
