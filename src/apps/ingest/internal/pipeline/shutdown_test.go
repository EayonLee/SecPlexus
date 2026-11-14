package pipeline_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connector"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

// strictPublisher 只在 ctx 有效时接受发布，模拟真实 Kafka 客户端。
type strictPublisher struct {
	mu        sync.Mutex
	published int
	failAll   bool
}

func (s *strictPublisher) Publish(ctx context.Context, events []pipeline.Event) error {
	if s.failAll {
		return errors.New("broker down")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.published += len(events)
	return nil
}

func (s *strictPublisher) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.published
}

// 回归（P0-1）：停机时最后一批必须发出去。
// 原实现用已取消的 ctx 发布，必然失败并丢弃整批。
func TestShutdownDrainsLastBatch(t *testing.T) {
	publisher := &strictPublisher{}
	line := pipeline.New(pipeline.Options{
		Workers: 1, BatchSize: 100, FlushInterval: time.Hour, DrainTimeout: 3 * time.Second,
	}, publisher)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- line.Start(ctx) }()

	snapshot := connector.Snapshot{ID: "c1", Channel: connector.ChannelHTTP, Engine: jsonRule(t)}
	if err := line.Submit(context.Background(), snapshot, []byte(`{"src":"1.1.1.1"}`), time.Now(), pipeline.Meta{}); err != nil {
		t.Fatal(err)
	}

	// 等到进入批处理但不满批。
	time.Sleep(100 * time.Millisecond)

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("停机返回错误: %v", err)
	}

	if publisher.count() != 1 {
		t.Fatalf("停机后已发布 %d 条，期望 1（最后一批被丢弃）", publisher.count())
	}
}

// 回归（P0-2）：关停后再提交必须返回 ErrClosed，而不是 panic。
func TestSubmitAfterCloseReturnsErrClosed(t *testing.T) {
	publisher := &strictPublisher{}
	line := pipeline.New(pipeline.Options{Workers: 1}, publisher)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- line.Start(ctx) }()

	time.Sleep(50 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	snapshot := connector.Snapshot{ID: "c1", Channel: connector.ChannelHTTP, Engine: jsonRule(t)}

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("关停后提交触发 panic: %v", recovered)
		}
	}()

	err := line.Submit(context.Background(), snapshot, []byte("x"), time.Now(), pipeline.Meta{})
	if !errors.Is(err, pipeline.ErrClosed) {
		t.Fatalf("错误 = %v，期望 ErrClosed", err)
	}
}

// 回归（P1）：排空超时后必须丢弃并计数，不能永远卡住进程。
func TestDrainTimeoutDropsAndCounts(t *testing.T) {
	publisher := &strictPublisher{failAll: true}
	line := pipeline.New(pipeline.Options{
		Workers: 1, BatchSize: 1, FlushInterval: 10 * time.Millisecond, DrainTimeout: 200 * time.Millisecond,
	}, publisher)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- line.Start(ctx) }()

	snapshot := connector.Snapshot{ID: "c1", Channel: connector.ChannelHTTP, Engine: jsonRule(t)}
	if err := line.Submit(context.Background(), snapshot, []byte(`{"src":"1.1.1.1"}`), time.Now(), pipeline.Meta{}); err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("停机返回错误: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("排空超时后进程没有退出")
	}

	if stats := line.Stats(); stats.Dropped == 0 {
		t.Fatalf("丢弃未计数: %+v", stats)
	}
}

// 停机后 Flush 不应永久阻塞。
func TestFlushAfterShutdownReturns(t *testing.T) {
	publisher := &strictPublisher{}
	line := pipeline.New(pipeline.Options{Workers: 1}, publisher)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- line.Start(ctx) }()

	time.Sleep(30 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	flushCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()

	if err := line.Flush(flushCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Flush 错误 = %v", err)
	}
}
