package pipeline_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

// 流水线负责识别失败，发布器负责选择目标 topic。
func TestFailedEventIsMarkedWithDefaultPipelineOptions(t *testing.T) {
	line, publisher := start(t, pipeline.Options{Workers: 1, BatchSize: 1, FlushInterval: 20 * time.Millisecond})
	snapshot := input.Entry{ID: "c1", Protocol: input.ProtocolHTTP, Mapping: jsonRule(t)}

	if err := line.Submit(context.Background(), snapshot, []byte(`not json`), time.Now(), pipeline.Meta{}); err != nil {
		t.Fatal(err)
	}
	_ = line.Flush(context.Background())

	events := publisher.events()
	if len(events) != 1 || events[0].Outcome.Status != pipeline.StatusError {
		t.Fatalf("默认流水线选项没有标记解析失败事件: %#v", events)
	}
}

// 成功事件永远走主 topic。
func TestSuccessfulEventRemainsOK(t *testing.T) {
	line, publisher := start(t, pipeline.Options{
		Workers: 1, BatchSize: 1, FlushInterval: 20 * time.Millisecond,
	})
	snapshot := input.Entry{ID: "c1", Protocol: input.ProtocolHTTP, Mapping: jsonRule(t)}

	if err := line.Submit(context.Background(), snapshot, []byte(`{"src":"10.0.0.1"}`), time.Now(), pipeline.Meta{}); err != nil {
		t.Fatal(err)
	}
	_ = line.Flush(context.Background())

	if events := publisher.events(); len(events) != 1 || events[0].Outcome.Status != pipeline.StatusOK {
		t.Fatalf("成功事件被误判为死信: %#v", events)
	}
}

func TestFailureBodyOmitsDevicePayload(t *testing.T) {
	line, _ := start(t, pipeline.Options{})
	source := input.Entry{ID: "c1", Protocol: input.ProtocolHTTP, Mapping: jsonRule(t)}
	event := line.Parse(source, []byte("private-device-marker"), time.Now(), "")
	if event.Outcome.Status != pipeline.StatusError || bytes.Contains(event.Body, []byte("private-device-marker")) {
		t.Fatalf("invalid diagnostic envelope: %s", event.Body)
	}
}
