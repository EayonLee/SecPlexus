// Package kafkachannel 消费客户已有的 Kafka 总线。
//
// 契约（spec）：一个接入点 = 一组 brokers + 一个外部 topic + 一条解析规则；
// 消费组 spx-ingest-<input_id>，offset 从最早开始，**手动提交**——
// 只有该批事件已成功发布到事件 topic 才提交位移，否则重投，重启后不丢数据。
package kafkachannel

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/kafka"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// Consumer 管理所有 Kafka 接入点的消费循环。
type Consumer struct {
	registry  input.Lookup
	pipeline  *pipeline.Pipeline
	publisher pipeline.Publisher

	mu       sync.Mutex
	running  map[string]context.CancelFunc // 接入点 → 取消函数，用于取消消失的消费者
	workers  sync.WaitGroup
	interval time.Duration
	// security 是消费侧的传输与身份配置，来自全局 Kafka 配置。
	security kafka.Security
}

// Options 是消费参数。
type Options struct {
	// ReloadInterval 是接入点集合的刷新间隔；默认 30 秒。
	ReloadInterval time.Duration
	// Security 是消费侧的传输与身份配置。
	// 与生产侧共用全局配置：per-input 覆盖需要给 inputs 加安全列，属独立 schema 决策。
	Security kafka.Security
}

// New 创建消费者。
func New(registry input.Lookup, line *pipeline.Pipeline, publisher pipeline.Publisher, options Options) *Consumer {
	if options.ReloadInterval <= 0 {
		options.ReloadInterval = 30 * time.Second
	}

	return &Consumer{
		registry:  registry,
		pipeline:  line,
		publisher: publisher,
		running:   make(map[string]context.CancelFunc),
		interval:  options.ReloadInterval,
	}
}

// Start 为每个 Kafka 接入点启动一个消费循环，直到 ctx 取消。
func (c *Consumer) Start(ctx context.Context) error {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	defer c.workers.Wait()

	if err := c.sync(ctx); err != nil {
		logging.From(ctx).Error("Kafka consumer start failed", "event", "kafka.start_failed",
			"error", logging.Error(err))
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := c.sync(ctx); err != nil {
				logging.From(ctx).Error("Kafka consumer reload failed", "event", "kafka.reload_failed",
					"error", logging.Error(err))
			}
		}
	}
}

// sync 让消费循环集合与注册表一致：新增的启动，消失的取消。
// 只加不减会让已删除的接入点继续从客户总线拉数据，
// 发布出带不存在 input_id 的幽灵事件，并且泄漏 goroutine。
func (c *Consumer) sync(ctx context.Context) error {
	desired := make(map[string]input.Entry)
	for _, entry := range c.registry.KafkaSources() {
		desired[entry.ID] = entry
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// 先取消消失的。
	for id, cancel := range c.running {
		if _, keep := desired[id]; keep {
			continue
		}

		logging.From(ctx).Info("Kafka consumer stopped", "event", "kafka.consumer_removed", "input_id", id)
		cancel()
		delete(c.running, id)
	}

	// 再启动新增的。
	for id, entry := range desired {
		if _, exists := c.running[id]; exists {
			continue
		}

		consumeCtx, cancel := context.WithCancel(ctx)
		c.running[id] = cancel

		c.workers.Add(1)
		go func(entry input.Entry) {
			defer c.workers.Done()

			if err := c.consume(consumeCtx, entry); err != nil && consumeCtx.Err() == nil {
				logging.From(ctx).Error("Kafka consumer stopped", "event", "kafka.consumer_stopped",
					"input_id", entry.ID, "error", logging.Error(err))
			}
		}(entry)
	}

	return nil
}

// consume 是单个接入点的消费循环：拉取 → 解析 → 发布 → 提交位移。
// 提交发生在发布成功之后：顺序反了会在发布失败时丢消息。
func (c *Consumer) consume(ctx context.Context, entry input.Entry) error {
	options, err := consumerOptions(entry, c.security)
	if err != nil {
		return err
	}

	client, err := kgo.NewClient(options...)
	if err != nil {
		return err
	}

	defer client.Close()

	logging.From(ctx).Info("Kafka consumer started", "event", "kafka.consumer_started",
		"input_id", entry.ID, "topic", entry.Kafka.Topic)

	for {
		fetches := client.PollFetches(ctx)
		if fetches.IsClientClosed() || ctx.Err() != nil {
			return nil
		}

		if err := fetches.Err(); err != nil {
			return fmt.Errorf("poll failed: %w", err)
		}

		records := make([]*kgo.Record, 0, fetches.NumRecords())
		fetches.EachRecord(func(record *kgo.Record) { records = append(records, record) })

		if len(records) == 0 {
			continue
		}

		// 提交失败要重试而不是退出：一次瞬时抖动不该让接入点永久停摆，
		// 而此时数据已发布，重投只会产生重复（由 event_id 去重），
		// 比"停摆直到人工重启"代价小得多。
		if err := c.processBatch(ctx, entry, records, func(commitCtx context.Context, batch []*kgo.Record) error {
			return c.commit(commitCtx, client, batch)
		}); err != nil {
			return err
		}
	}
}

// processBatch 解析并发布一批记录，只有发布成功才调用 commit。
//
// 提交动作由调用方传入而不是直接持有客户端：这样"发布先于提交"这个
// 不丢数据的核心顺序，可以脱离真实 broker 被验证。
// publish 在 ctx 结束前不会返回错误（内部退避重试），因此这里返回的错误只来自 commit。
func (c *Consumer) processBatch(ctx context.Context, entry input.Entry, records []*kgo.Record, commit func(context.Context, []*kgo.Record) error) error {
	events := make([]pipeline.Event, 0, len(records))
	for _, record := range records {
		events = append(events, c.pipeline.Parse(entry, record.Value, record.Timestamp))
	}

	// 发布成功才提交；失败时调用方重投同一批，最多退避到 5 秒。
	if err := c.publish(ctx, events); err != nil {
		logging.From(ctx).Warn("Publish failed; offsets not committed", "event", "kafka.publish_failed",
			"input_id", entry.ID, "records", len(records), "error", logging.Error(err))

		return nil
	}

	return commit(ctx, records)
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

// commit 带退避重试地提交位移，直到成功或进程退出。
func (c *Consumer) commit(ctx context.Context, client *kgo.Client, records []*kgo.Record) error {
	backoff := 100 * time.Millisecond

	for {
		err := client.CommitRecords(ctx, records...)
		if err == nil {
			return nil
		}

		// 关停时结束重试：位移没提交，重启后会重读这批消息并重投，
		// 由 event_id 去重兜底。返回 ctx 错误让调用方区分"关停"与"故障"。
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
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

// consumerOptions 拼装单个接入点的客户端选项。
//
// 单独成函数是为了能直接测试：真实消费循环需要 broker，
// 而"安全选项有没有接上"是必须被钉住的行为。
func consumerOptions(entry input.Entry, security kafka.Security) ([]kgo.Opt, error) {
	securityOptions, err := kafka.Options(security)
	if err != nil {
		return nil, err
	}

	options := []kgo.Opt{
		kgo.SeedBrokers(entry.Kafka.Brokers...),
		kgo.ConsumeTopics(entry.Kafka.Topic),
		kgo.ConsumerGroup("spx-ingest-" + entry.ID),
		// 首启从最早开始：接入点启用前积压的消息也要收进来。
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		// 手动提交：自动提交会在发布失败时把位移推进，直接丢数据。
		kgo.DisableAutoCommit(),
	}

	return append(options, securityOptions...), nil
}
