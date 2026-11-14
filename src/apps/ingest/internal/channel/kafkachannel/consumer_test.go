package kafkachannel_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/kafkachannel"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connector"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

// countingPublisher 记录发布次数。
type countingPublisher struct {
	mu    sync.Mutex
	calls int
}

func (c *countingPublisher) Publish(context.Context, []pipeline.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.calls++
	return nil
}

// 只绑定完整的 Kafka 接入点才会被消费；不完整的被跳过。
func TestKafkaSourcesOnlyReturnsUsableBindings(t *testing.T) {
	registry := connector.NewMemory()
	registry.Replace([]connector.Snapshot{
		{ID: "complete", Channel: connector.ChannelKafka, Kafka: connector.KafkaBinding{
			Brokers: []string{"127.0.0.1:9092"}, Topic: "external-events",
		}},
		{ID: "no-topic", Channel: connector.ChannelKafka, Kafka: connector.KafkaBinding{
			Brokers: []string{"127.0.0.1:9092"},
		}},
		{ID: "http", Channel: connector.ChannelHTTP, Token: connector.DigestToken("t")},
	})

	sources := registry.KafkaSources()
	if len(sources) != 1 || sources[0].ID != "complete" {
		t.Fatalf("sources = %#v", sources)
	}
}

// 不可达的 broker 不会让 Start 卡死：消费循环启动失败只记日志。
func TestStartToleratesUnreachableBroker(t *testing.T) {
	engine, err := normalizer.Compile(normalizer.Rule{
		ID:        "kafka-test",
		Parse:     parser.Spec{Mode: parser.ModeJSON},
		FieldMap:  map[string]string{"src": "src_endpoint.ip"},
		Constants: map[string]any{"class_uid": 2004},
	})
	if err != nil {
		t.Fatal(err)
	}

	registry := connector.NewMemory()
	registry.Replace([]connector.Snapshot{{
		ID: "conn-1", Channel: connector.ChannelKafka, Engine: engine,
		Kafka: connector.KafkaBinding{Brokers: []string{"127.0.0.1:1"}, Topic: "t"},
	}})

	publisher := &countingPublisher{}
	line := pipeline.New(pipeline.Options{Workers: 1}, publisher)

	consumer := kafkachannel.New(registry, line, publisher, kafkachannel.Options{ReloadInterval: time.Hour})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- consumer.Start(ctx) }()

	// 给消费循环一点时间尝试连接，然后确认 Start 仍在运行（没有 panic、没有阻塞退出）。
	time.Sleep(200 * time.Millisecond)

	select {
	case err := <-done:
		t.Fatalf("Start returned early: %v", err)
	default:
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not stop after cancellation")
	}
}
