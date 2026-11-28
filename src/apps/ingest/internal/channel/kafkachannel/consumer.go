// Package kafkachannel 消费客户已有的 Kafka 总线。
//
// 契约（spec）：一个接入点 = 一组 brokers + 一个外部 topic + 一条解析规则；
// 消费组 spx-ingest-<input_id>，按接入配置确定起始位置，**手动提交**——
// 只有该批事件已成功发布到事件 topic 才提交位移，否则重投，重启后不丢数据。
package kafkachannel

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connection/kafka"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/enum"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// Consumer 管理所有 Kafka 接入点的消费循环。
type Consumer struct {
	registry        input.Lookup
	pipeline        *pipeline.Pipeline
	publisher       pipeline.Publisher
	execution       input.KafkaExecutionStore
	offsetsRejected atomic.Uint64

	mu       sync.Mutex
	running  map[string]worker // 接入点 → 取消函数，用于取消消失的消费者
	workers  sync.WaitGroup
	interval time.Duration
	// security 是消费侧的传输与身份配置，来自全局 Kafka 配置。
	credentials map[string]kafka.SourceCredential
}

// OffsetsRejected 返回此实例因不一致位移停止消费分区的次数。
func (c *Consumer) OffsetsRejected() uint64 {
	return c.offsetsRejected.Load()
}

type worker struct {
	ctx        context.Context
	cancel     context.CancelFunc
	ready      *atomic.Bool
	blocked    *atomic.Bool
	processing *atomic.Pointer[input.Entry]
}

// Options 是消费参数。
type Options struct {
	// Execution 固定每批使用的历史配置；常驻服务必须使用持久存储。
	Execution input.KafkaExecutionStore
	// ReloadInterval 是接入点集合的刷新间隔；默认 30 秒。
	ReloadInterval time.Duration
	Credentials    map[string]kafka.SourceCredential // 按数据接入引用选择，空引用使用明文。
}

// New 创建消费者。
func New(registry input.Lookup, line *pipeline.Pipeline, publisher pipeline.Publisher, options Options) *Consumer {
	if options.ReloadInterval <= 0 {
		options.ReloadInterval = 30 * time.Second
	}

	return &Consumer{
		registry:    registry,
		pipeline:    line,
		publisher:   publisher,
		execution:   options.Execution,
		running:     make(map[string]worker),
		interval:    options.ReloadInterval,
		credentials: options.Credentials,
	}
}

// Activation 报告主题可用性、旧版本排空和需要处理的位移错误。
func (c *Consumer) Activation(entry input.Entry) input.ChannelActivation {
	c.mu.Lock()
	active, ok := c.running[entry.ID]
	c.mu.Unlock()
	if !ok || active.ctx.Err() != nil {
		return input.ChannelActivation{}
	}
	if active.blocked.Load() {
		return input.ChannelActivation{Problem: input.KafkaPositionUnavailable}
	}
	current := active.processing.Load()
	if current != nil && (current.Revision != entry.Revision || current.ParsingRuleVersion != entry.ParsingRuleVersion) {
		return input.ChannelActivation{Draining: true}
	}
	return input.ChannelActivation{Online: active.ready.Load()}
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
	for id, active := range c.running {
		if _, keep := desired[id]; keep {
			continue
		}

		logging.From(ctx).Info("Kafka consumer stopped", "event", "kafka.consumer_removed", "input_id", id)
		active.cancel()
		delete(c.running, id)
	}

	// 再启动新增的。
	for id, entry := range desired {
		if _, exists := c.running[id]; exists {
			continue
		}

		consumeCtx, cancel := context.WithCancel(ctx)
		runtime := worker{
			ctx: consumeCtx, cancel: cancel, ready: &atomic.Bool{}, blocked: &atomic.Bool{}, processing: &atomic.Pointer[input.Entry]{},
		}
		c.running[id] = runtime

		c.workers.Add(1)
		go func(entry input.Entry) {
			defer c.workers.Done()
			defer func() {
				cancel()
				c.mu.Lock()
				defer c.mu.Unlock()
				if c.running[entry.ID].ctx == consumeCtx {
					delete(c.running, entry.ID)
				}
			}()

			if err := c.consume(consumeCtx, entry, runtime); err != nil && consumeCtx.Err() == nil {
				logging.From(ctx).Error("Kafka consumer stopped", "event", "kafka.consumer_stopped",
					"input_id", entry.ID, "error", logging.Error(err))
			}
		}(entry)
	}

	return nil
}

// consume 是单个接入点的消费循环：拉取 → 解析 → 发布 → 提交位移。
// 提交发生在发布成功之后：顺序反了会在发布失败时丢消息。
func (c *Consumer) consume(ctx context.Context, entry input.Entry, runtime worker) error {
	profile, exists := c.credentials[entry.Kafka.Credential]
	if entry.Kafka.Credential != "" && (!exists || !profile.Allows(entry.Kafka.Brokers)) {
		return errors.New("could not find the Kafka data source credential")
	}
	options, err := consumerOptions(entry, profile.Security)
	if err != nil {
		return err
	}

	client, err := kgo.NewClient(options...)
	if err != nil {
		return err
	}

	defer client.CloseAllowingRebalance()
	check, cancel := context.WithTimeout(ctx, 10*time.Second)
	metadata := kmsg.NewPtrMetadataRequest()
	metadata.Topics = []kmsg.MetadataRequestTopic{{Topic: &entry.Kafka.Topic}}
	connection, err := metadata.RequestWith(check, client)
	cancel()
	if err != nil || len(connection.Topics) != 1 || connection.Topics[0].ErrorCode != 0 || len(connection.Topics[0].Partitions) == 0 {
		return errors.New("could not find the Kafka consumer topic")
	}
	runtime.ready.Store(true)
	defer runtime.ready.Store(false)

	logging.From(ctx).Info("Kafka consumer started", "event", "kafka.consumer_started",
		"input_id", entry.ID, "topic", entry.Kafka.Topic)

	for {
		fetches := client.PollRecords(ctx, maxBatchRecords)
		if fetches.IsClientClosed() || ctx.Err() != nil {
			return nil
		}

		for _, failure := range fetches.Errors() {
			if errors.Is(failure.Err, kerr.OffsetOutOfRange) {
				client.PauseFetchPartitions(map[string][]int32{failure.Topic: {failure.Partition}})
				runtime.blocked.Store(true)
				c.offsetsRejected.Add(1)
				logging.From(ctx).Warn("Kafka partition offset is outside retention",
					"event", "kafka.partition.offset_unavailable", "input_id", entry.ID, "partition", failure.Partition)
				continue
			}
			return fmt.Errorf("poll failed: %w", failure.Err)
		}

		state := fetchState{client: client, topicIDs: make(map[string]uuid.UUID), blocked: runtime.blocked, processing: runtime.processing}
		records := make([]*kgo.Record, 0, fetches.NumRecords())
		fetches.EachTopic(func(topic kgo.FetchTopic) {
			state.topicIDs[topic.Topic] = uuid.UUID(topic.TopicID)
			topic.EachRecord(func(record *kgo.Record) { records = append(records, record) })
		})

		if len(records) == 0 {
			client.AllowRebalance()
			continue
		}

		// 同一消费组内按批次取最新的来源与规则，停用后保留未提交位移。
		source, ok := c.registry.ByID(entry.ID)
		if !ok {
			return nil
		}
		// 超时后关闭客户端，下一次刷新从已提交的位置重新拉取。
		delivery, stop := context.WithTimeout(ctx, 30*time.Second)
		runtime.processing.Store(&source)
		err := c.processBatch(delivery, source, records, &state, func(commitCtx context.Context, batch []*kgo.Record) error {
			return c.commit(commitCtx, client, batch)
		})
		runtime.processing.Store(nil)
		stop()
		client.AllowRebalance()
		if err != nil {
			return err
		}
	}
}

const (
	maxBatchRecords = 500
	maxBatchBytes   = 4 << 20
)

type fetchState struct {
	client     *kgo.Client
	topicIDs   map[string]uuid.UUID
	blocked    *atomic.Bool
	processing *atomic.Pointer[input.Entry]
}

// processBatch 将拉取记录拆成有限的批次，并在发布前固定分区配置。
func (c *Consumer) processBatch(ctx context.Context, entry input.Entry, records []*kgo.Record, state *fetchState, commit func(context.Context, []*kgo.Record) error) error {
	events := make([]pipeline.Event, 0, min(len(records), maxBatchRecords))
	for len(records) > 0 && ctx.Err() == nil {
		var count, bytes int
		for _, record := range records[:min(maxBatchRecords, len(records))] {
			if count > 0 && bytes+len(record.Value) > maxBatchBytes {
				break
			}
			count++
			bytes += len(record.Value)
		}
		batch := records[:count]
		records = records[count:]
		boundaries, rejected, err := c.bind(ctx, entry, batch, state)
		if err != nil {
			return err
		}
		accepted := batch[:0]
		for _, record := range batch {
			if rejected[record.Partition] {
				continue
			}
			source := entry
			for _, boundary := range boundaries[record.Partition] {
				if boundary.Offset > record.Offset {
					break
				}
				source = boundary.Source
			}
			if state != nil && state.topicIDs[record.Topic] != uuid.Nil {
				sourceID, eventID := kafkaIdentities(source, state.topicIDs[record.Topic], record)
				events = append(events, c.pipeline.ParseKafka(source, record.Value, record.Timestamp, eventID, sourceID))
			} else {
				// 直接测试记录没有 broker 的主题身份。
				id := uuid.NewSHA1(uuid.NameSpaceOID, fmt.Appendf(nil, "spx-kafka/%s/%s/%d/%d/%d/%x",
					entry.ID, record.Topic, record.Partition, record.Offset, record.Timestamp.UnixMilli(), sha256.Sum256(record.Value)))
				events = append(events, c.pipeline.Parse(source, record.Value, record.Timestamp, id.String()))
			}
			accepted = append(accepted, record)
		}
		if len(accepted) == 0 {
			continue
		}

		if err := c.publish(ctx, events); err != nil {
			logging.From(ctx).Warn("Publish failed; offsets not committed", "event", "kafka.publish_failed",
				"input_id", entry.ID, "records", len(events), "error", logging.Error(err))
			return err
		}
		if err := commit(ctx, accepted); err != nil {
			return err
		}

		clear(events)
		events = events[:0]
	}

	return ctx.Err()
}

func kafkaIdentities(source input.Entry, topic uuid.UUID, record *kgo.Record) (string, string) {
	origin := uuid.NewSHA1(uuid.NameSpaceOID, fmt.Appendf(nil, "spx-kafka-source/%s/%s/%d/%d",
		source.ID, topic, record.Partition, record.Offset))
	execution := uuid.NewSHA1(origin, fmt.Appendf(nil, "v1/%d/%s/%d", source.Revision, source.ParsingRuleID, source.ParsingRuleVersion))
	return origin.String(), execution.String()
}

func (c *Consumer) bind(ctx context.Context, entry input.Entry, records []*kgo.Record, state *fetchState) (map[int32][]input.KafkaBoundary, map[int32]bool, error) {
	if c.execution == nil {
		return nil, nil, nil
	}
	if state == nil || state.topicIDs[entry.Kafka.Topic] == uuid.Nil {
		return nil, nil, errors.New("could not read the topic identity from the Kafka broker")
	}

	committed := state.client.CommittedOffsets()[entry.Kafka.Topic]
	windows := make(map[int32]input.KafkaWindow)
	for _, record := range records {
		window, exists := windows[record.Partition]
		if !exists {
			window = input.KafkaWindow{Partition: record.Partition, First: record.Offset, Committed: -1}
			if offset, ok := committed[record.Partition]; ok {
				window.Committed = offset.Offset
			}
		}
		window.Last = record.Offset
		windows[record.Partition] = window
	}

	boundaries := make(map[int32][]input.KafkaBoundary, len(windows))
	rejected := make(map[int32]bool)
	topicID := state.topicIDs[entry.Kafka.Topic].String()
	for partition, window := range windows {
		resolved, err := c.execution.Bind(ctx, entry, topicID, window)
		if errors.Is(err, input.ErrKafkaPosition) {
			rejected[partition] = true
			state.blocked.Store(true)
			state.client.PauseFetchPartitions(map[string][]int32{entry.Kafka.Topic: {partition}})
			c.offsetsRejected.Add(1)
			logging.From(ctx).Warn("Kafka partition requires an explicit reset",
				"event", "kafka.partition.offset_unavailable", "input_id", entry.ID, "partition", partition)
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		boundaries[partition] = resolved
		for _, boundary := range resolved {
			if boundary.Source.Revision != entry.Revision || boundary.Source.ParsingRuleVersion != entry.ParsingRuleVersion {
				// 历史批次完成提交之前，运行节点不能宣告新规则已生效。
				state.processing.Store(&boundary.Source)
				break
			}
		}
	}
	return boundaries, rejected, nil
}

// publish 带退避重试，直到成功或进程退出。
func (c *Consumer) publish(ctx context.Context, events []pipeline.Event) error {
	backoff := 100 * time.Millisecond

	for {
		pending, err := pipeline.PublishPending(ctx, c.publisher, events)
		events = pending
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
	start := kgo.NewOffset().AtStart()
	if entry.Kafka.StartPosition == enum.KafkaStartSinceCreated {
		// 用创建时保存的时间选择首位移，进程重启和重新分配不会移动该下界。
		start = kgo.NewOffset().AfterMilli(entry.Kafka.CreatedAt.UnixMilli())
	}

	options := []kgo.Opt{
		kgo.SeedBrokers(entry.Kafka.Brokers...),
		kgo.ConsumeTopics(entry.Kafka.Topic),
		kgo.ConsumerGroup("spx-ingest-" + entry.ID),
		kgo.ConsumeStartOffset(start),
		kgo.ConsumeResetOffset(kgo.NoResetOffset()),
		// 手动提交：自动提交会在发布失败时把位移推进，直接丢数据。
		kgo.DisableAutoCommit(),
		// 发布确认和位移提交完成后，再交出当前分区。
		kgo.BlockRebalanceOnPoll(),
		kgo.FetchMaxBytes(maxBatchBytes),
		kgo.MaxConcurrentFetches(1),
	}

	return append(options, securityOptions...), nil
}
