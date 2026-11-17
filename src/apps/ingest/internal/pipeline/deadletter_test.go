package pipeline_test

import (
	"context"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

// 配置了死信 topic 时，解析失败事件必须被标记为死信。
func TestFailedEventIsMarkedDeadLetter(t *testing.T) {
	line, publisher := start(t, pipeline.Options{
		Workers: 1, BatchSize: 1, FlushInterval: 20 * time.Millisecond,
		ParseFailureTopic: "spx.ingest.deadletter",
	})
	snapshot := input.Entry{ID: "c1", Protocol: input.ProtocolHTTP, Mapping: jsonRule(t)}

	if err := line.Submit(context.Background(), snapshot, []byte(`not json`), time.Now(), pipeline.Meta{}); err != nil {
		t.Fatal(err)
	}
	_ = line.Flush(context.Background())

	events := publisher.events()
	if len(events) != 1 {
		t.Fatalf("事件数 = %d", len(events))
	}
	if !events[0].DeadLetter {
		t.Fatal("解析失败事件没有被标记为死信")
	}
}

// 没配置死信 topic 时保持旧语义：仍发主 topic，只靠 parse.outcome 区分。
func TestFailedEventStaysOnMainTopicWithoutDeadLetterConfig(t *testing.T) {
	line, publisher := start(t, pipeline.Options{Workers: 1, BatchSize: 1, FlushInterval: 20 * time.Millisecond})
	snapshot := input.Entry{ID: "c1", Protocol: input.ProtocolHTTP, Mapping: jsonRule(t)}

	if err := line.Submit(context.Background(), snapshot, []byte(`not json`), time.Now(), pipeline.Meta{}); err != nil {
		t.Fatal(err)
	}
	_ = line.Flush(context.Background())

	events := publisher.events()
	if len(events) != 1 || events[0].DeadLetter {
		t.Fatalf("未配置死信时不应分流: %#v", events)
	}
}

// 成功事件永远走主 topic。
func TestSuccessfulEventIsNeverDeadLetter(t *testing.T) {
	line, publisher := start(t, pipeline.Options{
		Workers: 1, BatchSize: 1, FlushInterval: 20 * time.Millisecond,
		ParseFailureTopic: "spx.ingest.deadletter",
	})
	snapshot := input.Entry{ID: "c1", Protocol: input.ProtocolHTTP, Mapping: jsonRule(t)}

	if err := line.Submit(context.Background(), snapshot, []byte(`{"src":"10.0.0.1"}`), time.Now(), pipeline.Meta{}); err != nil {
		t.Fatal(err)
	}
	_ = line.Flush(context.Background())

	if events := publisher.events(); len(events) != 1 || events[0].DeadLetter {
		t.Fatalf("成功事件被误判为死信: %#v", events)
	}
}
