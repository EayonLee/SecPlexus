// Package publish 把已解析的事件写到 Kafka。
//
// Kafka 是第一版唯一缓冲层：事件不落盘，acks 与重试语义直接决定是否丢数据。
package publish

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/EayonLee/SecPlexus/src/packages/go/logging"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/kafka"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

// Options 是 Kafka 生产者参数。
type Options struct {
	Brokers           []string       // broker 地址列表。
	EventTopic        string         // 归一化成功事件的 topic。
	ParseFailureTopic string         // 解析失败事件的 topic。
	Security          kafka.Security // 传输与身份安全配置。
	Timeout           time.Duration  // 单次生产的超时；默认 10 秒。
}

// closeFlushTimeout 是关停时等待在途事件的上限，需小于编排层的强制终止宽限期。
const closeFlushTimeout = 3 * time.Second

// Kafka 是 pipeline.Publisher 的 Kafka 实现。
type Kafka struct {
	client            *kgo.Client
	eventTopic        string
	parseFailureTopic string
	timeout           time.Duration
}

// NewKafka 创建生产者并校验连通性。
// 生产前先建立连接：启动期就发现 broker 不可达，好过第一条事件才发现。
func NewKafka(ctx context.Context, options Options) (*Kafka, error) {
	if len(options.Brokers) == 0 {
		return nil, errors.New("kafka brokers are required")
	}
	if options.EventTopic == "" {
		return nil, errors.New("kafka topic is required")
	}

	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	securityOptions, err := kafka.Options(options.Security)
	if err != nil {
		return nil, err
	}

	// 等所有同步副本确认：单副本确认在 broker 掉盘时会丢事件。
	// franz-go 默认就是 -1，这里显式钉住——数据持久性决策不依赖库的默认值。
	// 幂等写入同样是默认开启的，无需额外选项；重试不会造成重复。
	clientOptions := append([]kgo.Opt{
		kgo.SeedBrokers(options.Brokers...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.ProducerBatchMaxBytes(4 << 20),
		kgo.RecordRetries(5),
		kgo.ProduceRequestTimeout(timeout),
	}, securityOptions...)

	client, err := kgo.NewClient(clientOptions...)
	if err != nil {
		return nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err = client.Ping(pingCtx); err != nil {
		client.Close()

		return nil, fmt.Errorf("kafka is unreachable: %w", err)
	}

	return &Kafka{client: client, eventTopic: options.EventTopic, parseFailureTopic: options.ParseFailureTopic, timeout: timeout}, nil
}

// Publish 同步发送一批事件；返回 nil 表示全部已被 Kafka 确认。
// 用同步批量而不是异步回调：管道需要明确的成功信号才能记账，
// 失败时靠上层重试，而不是让事件悬在客户端缓冲区里。
func (k *Kafka) Publish(ctx context.Context, events []pipeline.Event) error {
	if len(events) == 0 {
		return nil
	}

	records := make([]*kgo.Record, 0, len(events))
	for index := range events {
		event := &events[index]
		records = append(records, &kgo.Record{
			Topic: k.topicFor(event),
			Key:   []byte(event.PartitionKey),
			Value: event.Body,
		})
	}

	produceCtx, cancel := context.WithTimeout(ctx, k.timeout)
	defer cancel()

	results := k.client.ProduceSync(produceCtx, records...)
	if err := results.FirstErr(); err != nil {
		return fmt.Errorf("kafka publish failed: %w", err)
	}

	return nil
}

// topicFor 选择事件应该发到哪个 topic。
//
// 单独成函数是为了能直接测试分流规则：构造真实 Kafka 客户端代价太高，
// 而"失败事件去哪"是必须被钉住的行为。
func (k *Kafka) topicFor(event *pipeline.Event) string {
	if event.DeadLetter && k.parseFailureTopic != "" {
		return k.parseFailureTopic
	}

	return k.eventTopic
}

// Close 冲刷未完成的生产请求并关闭连接。
// 关停阶段没有调用方可以报告失败，记录日志已经是它能做的最好的事。
// Close 送出在途事件后关闭客户端。
//
// Flush 必须有上限：broker 不可达时它会一直等，进程收不到 SIGTERM 的响应，
// 只能被 SIGKILL 结束——那正好发生在最需要它把缓冲刷出去的时候。
// 超时后放弃剩余记录并留下日志，让"丢了多少"可见，而不是静默卡死。
func (k *Kafka) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), closeFlushTimeout)
	defer cancel()

	if err := k.client.Flush(ctx); err != nil {
		logging.From(context.Background()).Warn("Kafka flush did not finish before shutdown",
			"event", "kafka.flush_incomplete", "error", logging.Error(err))
	}

	k.client.Close()
}
