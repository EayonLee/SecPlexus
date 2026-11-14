// Package kafkachannel 消费客户已有的 Kafka 总线。
//
// 契约（spec）：一个接入点 = 一组 brokers + 一个外部 topic + 一条解析规则；
// 消费组 spx-ingest-<connector_id>，offset 从最早开始，**手动提交**——
// 只有该批事件已成功发布到事件 topic 才提交位移，否则重投，重启后不丢数据。
package kafkachannel

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connector"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// Consumer 管理所有 Kafka 接入点的消费循环。
type Consumer struct {
	registry  connector.Registry
	pipeline  *pipeline.Pipeline
	publisher pipeline.Publisher

	mu       sync.Mutex
	workers  sync.WaitGroup
	interval time.Duration
}

// Options 是消费参数。
type Options struct {
	// ReloadInterval 是接入点集合的刷新间隔；默认 30 秒。
	ReloadInterval time.Duration
}

// New 创建消费者。
func New(registry connector.Registry, line *pipeline.Pipeline, publisher pipeline.Publisher, options Options) *Consumer {
	if options.ReloadInterval <= 0 {
		options.ReloadInterval = 30 * time.Second
	}

	return &Consumer{
		registry:  registry,
		pipeline:  line,
		publisher: publisher,
		interval:  options.ReloadInterval,
	}
}

// Start 为每个 Kafka 接入点启动一个消费循环，直到 ctx 取消。
func (c *Consumer) Start(ctx context.Context) error {
	running := make(map[string]struct{})

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	defer c.workers.Wait()

	if err := c.sync(ctx, running); err != nil {
		logging.From(ctx).Error("Kafka consumer start failed", "event", "kafka.start_failed",
			"error", logging.Error(err))
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := c.sync(ctx, running); err != nil {
				logging.From(ctx).Error("Kafka consumer reload failed", "event", "kafka.reload_failed",
					"error", logging.Error(err))
			}
		}
	}
}

// sync 为新增的 Kafka 接入点启动消费循环；已启动的保持运行。
func (c *Consumer) sync(ctx context.Context, running map[string]struct{}) error {
	for _, snapshot := range c.registry.KafkaSources() {
		if _, exists := running[snapshot.ID]; exists {
			continue
		}

		running[snapshot.ID] = struct{}{}

		c.mu.Lock()
		c.workers.Add(1)
		c.mu.Unlock()

		go func(snapshot connector.Snapshot) {
			defer c.workers.Done()

			if err := c.consume(ctx, snapshot); err != nil && ctx.Err() == nil {
				logging.From(ctx).Error("Kafka consumer stopped", "event", "kafka.consumer_stopped",
					"connector_id", snapshot.ID, "error", logging.Error(err))
			}
		}(snapshot)
	}

	return nil
}

// consume 是单个接入点的消费循环：拉取 → 解析 → 发布 → 提交位移。
// 提交发生在发布成功之后：顺序反了会在发布失败时丢消息。
func (c *Consumer) consume(ctx context.Context, snapshot connector.Snapshot) error {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(snapshot.Kafka.Brokers...),
		kgo.ConsumeTopics(snapshot.Kafka.Topic),
		kgo.ConsumerGroup("spx-ingest-"+snapshot.ID),
		// 首启从最早开始：接入点启用前积压的消息也要收进来。
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		// 手动提交：自动提交会在发布失败时把位移推进，直接丢数据。
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return err
	}

	defer client.Close()

	logging.From(ctx).Info("Kafka consumer started", "event", "kafka.consumer_started",
		"connector_id", snapshot.ID, "topic", snapshot.Kafka.Topic)

	for {
		fetches := client.PollFetches(ctx)
		if fetches.IsClientClosed() || ctx.Err() != nil {
			return nil
		}

		if err := fetches.Err(); err != nil {
			return fmt.Errorf("poll failed: %w", err)
		}

		records := make([]*kgo.Record, 0, fetches.NumRecords())
		events := make([]pipeline.Event, 0, fetches.NumRecords())

		fetches.EachRecord(func(record *kgo.Record) {
			records = append(records, record)
			events = append(events, c.pipeline.Parse(snapshot, record.Value, record.Timestamp))
		})

		if len(events) == 0 {
			continue
		}

		// 发布成功才提交；失败时重投同一批，最多退避到 5 秒。
		if err := c.publish(ctx, events); err != nil {
			logging.From(ctx).Warn("Publish failed; offsets not committed", "event", "kafka.publish_failed",
				"connector_id", snapshot.ID, "records", len(records), "error", logging.Error(err))

			continue
		}

		if err := client.CommitRecords(ctx, records...); err != nil {
			if ctx.Err() != nil {
				return nil
			}

			return fmt.Errorf("commit failed: %w", err)
		}
	}
}

// publish 带退避重试，直到成功或进程退出。
func (c *Consumer) publish(ctx context.Context, events []pipeline.Event) error {
	backoff := 100 * time.Millisecond

	for {
		err := c.publisher.Publish(ctx, events)
		if err == nil {
			return nil
		}

		if ctx.Err() != nil {
			return errors.Join(ctx.Err(), err)
		}

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return ctx.Err()
		}

		if backoff < 5*time.Second {
			backoff *= 2
		}
	}
}
