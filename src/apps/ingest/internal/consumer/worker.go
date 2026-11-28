// Package consumer 消费标准 Event 并在存储确认后提交 Kafka 位移。
package consumer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository/event"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// Worker 独占一个消费客户端。调用方负责配置分组、关闭自动提交及回收客户端。
type Worker struct {
	Kafka         *kgo.Client
	Store         *event.Store
	Projector     *event.Projector
	FailureTopic  string
	MaxRecords    int
	MaxBytes      int
	BatchTimeout  time.Duration
	CheckInterval time.Duration
}

// Run 在完成写入或隔离后提交处理过的位移。依赖失败即返回，重启时从已提交位移恢复。
func (w Worker) Run(ctx context.Context) error {
	if err := w.Store.CheckSchema(ctx); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("check event table: %w", err)
	}
	nextCheck := time.Now().Add(w.CheckInterval)
	paused := make(map[int32]bool)
	for ctx.Err() == nil {
		fetchContext, cancel := context.WithDeadline(ctx, nextCheck)
		fetches := w.Kafka.PollRecords(fetchContext, w.MaxRecords)
		checkDue := errors.Is(fetchContext.Err(), context.DeadlineExceeded)
		cancel()
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if checkDue || !time.Now().Before(nextCheck) {
			if err := w.Store.CheckSchema(ctx); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("check event table: %w", err)
			}
			nextCheck = time.Now().Add(w.CheckInterval)
		}
		if failures := fetches.Errors(); len(failures) != 0 {
			// 同批异常共享排查期限，避免大量坏分区拖住仍可写入的分区。
			check, finish := context.WithTimeout(ctx, 2*time.Second)
			for _, failure := range failures {
				if errors.Is(failure.Err, kerr.OffsetOutOfRange) {
					if !paused[failure.Partition] {
						paused[failure.Partition] = true
						w.Kafka.PauseFetchPartitions(map[string][]int32{failure.Topic: {failure.Partition}})
						w.reportUnavailableOffset(check, failure.Topic, failure.Partition)
					}
					continue
				}
				if !checkDue || !errors.Is(failure.Err, context.DeadlineExceeded) {
					finish()
					return fmt.Errorf("fetch events for storage: %w", failure.Err)
				}
			}
			finish()
		}
		records := fetches.Records()
		if len(paused) != 0 {
			kept := records[:0]
			for _, record := range records {
				if !paused[record.Partition] {
					kept = append(kept, record)
				}
			}
			records = kept
		}
		if len(records) == 0 {
			w.Kafka.AllowRebalance()
			continue
		}

		batch, stop := context.WithTimeout(ctx, w.BatchTimeout)
		err := w.storeBatch(batch, records)
		stop()
		w.Kafka.AllowRebalance()
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// reportUnavailableOffset 查询 broker 的保留边界。失败时仍输出已确认位移和暂停事实。
func (w Worker) reportUnavailableOffset(ctx context.Context, topic string, partition int32) {
	committed := int64(-1)
	if offset, ok := w.Kafka.CommittedOffsets()[topic][partition]; ok {
		committed = offset.Offset
	}
	lookup := func(timestamp int64) int64 {
		request := kmsg.NewPtrListOffsetsRequest()
		item := kmsg.NewListOffsetsRequestTopicPartition()
		item.Partition, item.Timestamp = partition, timestamp
		request.Topics = []kmsg.ListOffsetsRequestTopic{{Topic: topic, Partitions: []kmsg.ListOffsetsRequestTopicPartition{item}}}
		response, err := request.RequestWith(ctx, w.Kafka)
		if err != nil || len(response.Topics) != 1 || len(response.Topics[0].Partitions) != 1 {
			return -1
		}
		result := response.Topics[0].Partitions[0]
		if result.ErrorCode != 0 {
			return -1
		}
		return result.Offset
	}
	logging.From(ctx).Warn("Event partition paused because its offset is outside retention",
		"event", "ingest.events.offset_unavailable", "topic", topic, "partition", partition,
		"committed_offset", committed, "earliest_offset", lookup(-2), "end_offset", lookup(-1))
}

type rejectedEvent struct {
	Topic     string `json:"topic"`
	Partition int32  `json:"partition"`
	Offset    int64  `json:"offset"`
	Reason    string `json:"reason"`
	Bytes     int    `json:"bytes"` // 原消息字节数；用 Kafka 坐标回查。
}

func (w Worker) storeBatch(ctx context.Context, records []*kgo.Record) error {
	for len(records) > 0 {
		var body bytes.Buffer
		encoder := json.NewEncoder(&body)
		var rejects []*kgo.Record
		var used, end int
		for end < len(records) {
			record := records[end]
			if end > 0 && used+len(record.Value) > w.MaxBytes {
				break
			}
			row, err := w.Projector.Project(record.Value)
			if err != nil {
				value, marshalErr := json.Marshal(rejectedEvent{
					Topic: record.Topic, Partition: record.Partition, Offset: record.Offset,
					Reason: "invalid_event_projection", Bytes: len(record.Value),
				})
				if marshalErr != nil {
					return fmt.Errorf("encode rejected event: %w", marshalErr)
				}
				key := fmt.Sprintf("%s/%d/%d", record.Topic, record.Partition, record.Offset)
				rejects = append(rejects, &kgo.Record{Topic: w.FailureTopic, Key: []byte(key), Value: value})
			} else if err := encoder.Encode(row); err != nil {
				return fmt.Errorf("encode event batch: %w", err)
			}
			used += len(record.Value)
			end++
		}

		if err := w.Store.Insert(ctx, body.Bytes()); err != nil {
			return fmt.Errorf("store event batch: %w", err)
		}
		if len(rejects) != 0 {
			if err := w.Kafka.ProduceSync(ctx, rejects...).FirstErr(); err != nil {
				return fmt.Errorf("publish event failures: %w", err)
			}
			logging.From(ctx).Warn("Events sent to failure topic", "event", "ingest.events.quarantined", "count", len(rejects))
		}
		if err := w.Kafka.CommitRecords(ctx, records[:end]...); err != nil {
			return fmt.Errorf("commit event offsets: %w", err)
		}
		logging.From(ctx).Debug("Event offsets committed", "event", "ingest.events.batch_committed", "stored", end-len(rejects))
		records = records[end:]
	}
	return nil
}
