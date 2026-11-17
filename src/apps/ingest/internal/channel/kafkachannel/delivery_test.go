package kafkachannel

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

// 契约：只有事件已成功发布，才允许提交位移。
// 顺序反了会在发布失败时丢消息，而重启后无法补偿——这是整条管道最贵的一个 bug。
func TestProcessBatchCommitsOnlyAfterPublishSucceeds(t *testing.T) {
	batch := []*kgo.Record{
		{Topic: "external", Value: []byte(`{"src":"10.0.0.1"}`)},
		{Topic: "external", Value: []byte(`{"src":"10.0.0.2"}`)},
	}

	var committed atomic.Int64

	consumer := newConsumer(t, &countingPublisher{})
	err := consumer.processBatch(context.Background(), input.Entry{ID: "kafka-1"}, batch,
		func(_ context.Context, records []*kgo.Record) error {
			committed.Add(int64(len(records)))

			return nil
		})
	if err != nil {
		t.Fatalf("processBatch = %v", err)
	}

	if committed.Load() != 2 {
		t.Fatalf("提交了 %d 条位移，期望 2 条", committed.Load())
	}
}

// 发布失败时绝不能提交：未提交的位移会在重启后重新投递，数据不丢。
func TestProcessBatchSkipsCommitWhenPublishFails(t *testing.T) {
	batch := []*kgo.Record{{Topic: "external", Value: []byte(`{"src":"10.0.0.1"}`)}}

	var committed atomic.Int64

	// 发布持续失败，ctx 在 50 毫秒后取消，模拟"总线一直不可写"。
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	consumer := newConsumer(t, &failingPublisher{err: errors.New("broker unreachable")})
	if err := consumer.processBatch(ctx, input.Entry{ID: "kafka-1"}, batch,
		func(_ context.Context, records []*kgo.Record) error {
			committed.Add(int64(len(records)))

			return nil
		}); err != nil {
		t.Fatalf("processBatch = %v", err)
	}

	if committed.Load() != 0 {
		t.Fatalf("发布失败仍提交了 %d 条位移", committed.Load())
	}
}

// 提交失败要把错误交回调用方，由它决定重试还是停摆；静默吞掉会悄悄重复消费。
func TestProcessBatchReturnsCommitError(t *testing.T) {
	batch := []*kgo.Record{{Topic: "external", Value: []byte(`{"src":"10.0.0.1"}`)}}
	wanted := errors.New("commit rejected")

	consumer := newConsumer(t, &countingPublisher{})
	err := consumer.processBatch(context.Background(), input.Entry{ID: "kafka-1"}, batch,
		func(context.Context, []*kgo.Record) error { return wanted })
	if !errors.Is(err, wanted) {
		t.Fatalf("processBatch = %v，期望提交错误", err)
	}
}

// 空批次不触发任何发布或提交。
func TestProcessBatchIgnoresEmptyBatch(t *testing.T) {
	var committed atomic.Int64
	publisher := &countingPublisher{}

	consumer := newConsumer(t, publisher)
	if err := consumer.processBatch(context.Background(), input.Entry{ID: "kafka-1"}, nil,
		func(_ context.Context, records []*kgo.Record) error {
			committed.Add(int64(len(records)))

			return nil
		}); err != nil {
		t.Fatalf("processBatch = %v", err)
	}

	if committed.Load() != 0 || publisher.batches.Load() != 1 {
		t.Fatalf("空批次不应提交（提交 %d，发布批次数 %d）", committed.Load(), publisher.batches.Load())
	}
}

func newConsumer(t *testing.T, publisher pipeline.Publisher) *Consumer {
	t.Helper()

	line := pipeline.New(pipeline.Options{Workers: 1, BatchSize: 1, FlushInterval: 10 * time.Millisecond}, publisher)

	return &Consumer{pipeline: line, publisher: publisher}
}

type countingPublisher struct{ batches atomic.Int64 }

func (p *countingPublisher) Publish(_ context.Context, _ []pipeline.Event) error {
	p.batches.Add(1)

	return nil
}

type failingPublisher struct{ err error }

func (p *failingPublisher) Publish(_ context.Context, _ []pipeline.Event) error { return p.err }
