//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
)

func TestKafkaExecutionRetryUsesOriginalBoundaryUntilCommitted(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()
	topicID := uuid.NewString()
	old := input.Entry{ID: uuid.NewString(), Name: "old", Protocol: input.ProtocolKafka, Revision: 1}
	next := old
	next.Name = "new"
	next.Revision = 2
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM ingest.kafka_execution_boundaries WHERE input_id=$1", old.ID)
		_, _ = pool.Exec(ctx, "DELETE FROM ingest.kafka_partition_heads WHERE input_id=$1", old.ID)
	})

	initial := input.KafkaWindow{Partition: 0, First: 20, Last: 25, Committed: -1}
	before := repository.NewKafkaExecutionStore(pool)
	if _, err := before.Bind(ctx, old, topicID, initial); err != nil {
		t.Fatal(err)
	}

	// 新进程读了新配置，但 broker 尚未确认上一次发布的位移。
	restarted := repository.NewKafkaExecutionStore(pool)
	retry, err := restarted.Bind(ctx, next, topicID, initial)
	if err != nil || len(retry) != 1 || retry[0].Source.Name != old.Name || retry[0].Offset != 20 {
		t.Fatalf("retried batch changed revision: %#v %v", retry, err)
	}
	following := input.KafkaWindow{Partition: 0, First: 26, Last: 30, Committed: -1}
	if _, err := restarted.Bind(ctx, next, topicID, following); !errors.Is(err, input.ErrKafkaPosition) {
		t.Fatalf("unconfirmed gap accepted: %v", err)
	}
	following.Committed = 26
	bound, err := restarted.Bind(ctx, next, topicID, following)
	if err != nil || len(bound) != 1 || bound[0].Source.Name != next.Name || bound[0].Offset != 26 {
		t.Fatalf("confirmed boundary did not activate: %#v %v", bound, err)
	}
	if _, err := restarted.Bind(ctx, next, uuid.NewString(), following); !errors.Is(err, input.ErrKafkaPosition) {
		t.Fatalf("replacement topic bypassed the recorded identity: %v", err)
	}

	// 即使客户端按授权恢复原来的消费位置，已存在的分区边界不会被改写。
	both := input.KafkaWindow{Partition: 0, First: 20, Last: 30, Committed: 20}
	history, err := restarted.Bind(ctx, next, topicID, both)
	if err != nil || len(history) != 2 || history[0].Source.Name != old.Name || history[1].Source.Name != next.Name {
		t.Fatalf("recorded history changed: %#v %v", history, err)
	}
}
