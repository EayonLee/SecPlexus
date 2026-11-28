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
	err := consumer.processBatch(context.Background(), input.Entry{ID: "kafka-1"}, batch, nil,
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

func TestProcessBatchCommitsInBoundedChunks(t *testing.T) {
	records := make([]*kgo.Record, 501)
	for offset := range records {
		records[offset] = &kgo.Record{Topic: "external", Offset: int64(offset), Value: []byte(`{"src":"10.0.0.1"}`)}
	}
	publisher := &countingPublisher{}
	consumer := newConsumer(t, publisher)
	var lastOffsets []int64
	if err := consumer.processBatch(context.Background(), input.Entry{ID: "kafka-1"}, records, nil,
		func(_ context.Context, batch []*kgo.Record) error {
			lastOffsets = append(lastOffsets, batch[len(batch)-1].Offset)
			return nil
		}); err != nil {
		t.Fatal(err)
	}
	if len(lastOffsets) != 2 || lastOffsets[0] != 499 || lastOffsets[1] != 500 || publisher.batches.Load() != 2 {
		t.Fatalf("期望两批按连续位移提交，实际位移 %v，发布批次 %d", lastOffsets, publisher.batches.Load())
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
	if err := consumer.processBatch(ctx, input.Entry{ID: "kafka-1"}, batch, nil,
		func(_ context.Context, records []*kgo.Record) error {
			committed.Add(int64(len(records)))

			return nil
		}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("发布超时应让消费循环退出并保留位移: %v", err)
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
	err := consumer.processBatch(context.Background(), input.Entry{ID: "kafka-1"}, batch, nil,
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
	if err := consumer.processBatch(context.Background(), input.Entry{ID: "kafka-1"}, nil, nil,
		func(_ context.Context, records []*kgo.Record) error {
			committed.Add(int64(len(records)))

			return nil
		}); err != nil {
		t.Fatalf("processBatch = %v", err)
	}

	if committed.Load() != 0 || publisher.batches.Load() != 0 {
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

func TestProcessBatchKeepsEventIdentityAcrossRestarts(t *testing.T) {
	received := time.Unix(1800000000, 0)
	batch := []*kgo.Record{
		{Topic: "external", Partition: 3, Offset: 40, Timestamp: received, Value: []byte("same")},
		{Topic: "external", Partition: 3, Offset: 41, Timestamp: received, Value: []byte("same")},
	}
	source := input.Entry{ID: "kafka-1", Protocol: input.ProtocolKafka}
	commitFailed := errors.New("commit rejected")
	first := &recordedDelivery{}
	err := newConsumer(t, first).processBatch(context.Background(), source, batch, nil,
		func(context.Context, []*kgo.Record) error { return commitFailed })
	if !errors.Is(err, commitFailed) {
		t.Fatal(err)
	}
	restarted := &recordedDelivery{}
	err = newConsumer(t, restarted).processBatch(context.Background(), source, batch, nil,
		func(context.Context, []*kgo.Record) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(first.ids) != 2 || len(restarted.ids) != 2 || first.ids[0] == first.ids[1] ||
		first.ids[0] != restarted.ids[0] || first.ids[1] != restarted.ids[1] {
		t.Fatalf("identities before and after restart: %v, %v", first.ids, restarted.ids)
	}
}

type recordedDelivery struct{ ids []string }

func (p *recordedDelivery) Publish(_ context.Context, events []pipeline.Event) error {
	for _, event := range events {
		p.ids = append(p.ids, event.EventID)
	}
	return nil
}

func TestProcessBatchRetriesOnlyUnconfirmedRecords(t *testing.T) {
	batch := []*kgo.Record{
		{Topic: "external", Offset: 40, Value: []byte("first")},
		{Topic: "external", Offset: 41, Value: []byte("second")},
	}
	publisher := &partialDelivery{}
	consumer := newConsumer(t, publisher)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var committed int
	err := consumer.processBatch(ctx, input.Entry{ID: "test"}, batch, nil,
		func(_ context.Context, input []*kgo.Record) error {
			if len(publisher.confirmed) != len(input) {
				t.Fatal("source offsets were committed before all events were confirmed")
			}
			committed += len(input)
			return nil
		})
	if err != nil || committed != 2 || len(publisher.confirmed) != 2 || publisher.confirmed[0] == publisher.confirmed[1] {
		t.Fatalf("unexpected result: commit count %d, confirmed %v, error %v", committed, publisher.confirmed, err)
	}
}

type partialDelivery struct{ confirmed []string }

func (p *partialDelivery) Publish(_ context.Context, events []pipeline.Event) error {
	if len(p.confirmed) == 0 {
		p.confirmed = append(p.confirmed, events[0].EventID)
		return &pipeline.PartialPublishError{Failed: []int{1}, Cause: errors.New("second record not confirmed")}
	}
	for _, event := range events {
		p.confirmed = append(p.confirmed, event.EventID)
	}
	return nil
}
