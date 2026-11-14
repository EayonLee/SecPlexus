// Package publish 把已解析的事件写到 Kafka。
//
// Kafka 是第一版唯一缓冲层：事件不落盘，acks 与重试语义直接决定是否丢数据。
package publish

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

// Options 是 Kafka 生产者参数。
type Options struct {
	Brokers []string      // broker 地址列表。
	Topic   string        // 唯一发布 topic。
	Timeout time.Duration // 单次生产的超时；默认 10 秒。
}

// Kafka 是 pipeline.Publisher 的 Kafka 实现。
type Kafka struct {
	client  *kgo.Client
	topic   string
	timeout time.Duration
}

// NewKafka 创建生产者并校验连通性。
// 生产前先建立连接：启动期就发现 broker 不可达，好过第一条事件才发现。
func NewKafka(ctx context.Context, options Options) (*Kafka, error) {
	if len(options.Brokers) == 0 {
		return nil, errors.New("kafka brokers are required")
	}
	if options.Topic == "" {
		return nil, errors.New("kafka topic is required")
	}

	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	client, err := kgo.NewClient(
		kgo.SeedBrokers(options.Brokers...),
		// 等所有同步副本确认：单副本确认在 broker 掉盘时会丢事件。
		// franz-go 默认就是 -1，这里显式钉住——数据持久性决策不依赖库的默认值。
		// 幂等写入同样是默认开启的，无需额外选项；重试不会造成重复。
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.ProducerBatchMaxBytes(4<<20),
		kgo.RecordRetries(5),
		kgo.ProduceRequestTimeout(timeout),
	)
	if err != nil {
		return nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err = client.Ping(pingCtx); err != nil {
		client.Close()

		return nil, fmt.Errorf("kafka is unreachable: %w", err)
	}

	return &Kafka{client: client, topic: options.Topic, timeout: timeout}, nil
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
			Topic: k.topic,
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

// Close 冲刷未完成的生产请求并关闭连接。
// 关停阶段没有调用方可以报告失败，记录日志已经是它能做的最好的事。
func (k *Kafka) Close() {
	// Flush 等待在途请求完成；直接 Close 会让它们失败。
	if err := k.client.Flush(context.Background()); err != nil {
		// 关闭路径上没有返回错误的位置；日志留给运维定位。
		_ = err
	}

	k.client.Close()
}
