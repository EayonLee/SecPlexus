package pipeline_test

import (
	"context"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

// 回归：计数必须能分到接入点，否则无法定位"哪台设备在发坏数据"。
func TestStatsCountPerInput(t *testing.T) {
	line, _ := start(t, pipeline.Options{Workers: 2, BatchSize: 4, FlushInterval: 10 * time.Millisecond})

	healthy := input.Entry{ID: "input-a", Protocol: input.ProtocolHTTP, Mapping: jsonRule(t)}
	broken := input.Entry{ID: "input-b", Protocol: input.ProtocolHTTP, Mapping: jsonRule(t)}

	if err := line.Submit(context.Background(), healthy, []byte(`{"src":"10.0.0.1"}`), time.Now(), pipeline.Meta{}); err != nil {
		t.Fatal(err)
	}
	if err := line.Submit(context.Background(), broken, []byte(`not json`), time.Now(), pipeline.Meta{}); err != nil {
		t.Fatal(err)
	}

	_ = line.Flush(context.Background())

	stats := line.Stats()

	first := stats.ByInput["input-a"]
	if first.Accepted != 1 || first.Parsed != 1 {
		t.Fatalf("input-a = %+v，期望 1/1", first)
	}

	second := stats.ByInput["input-b"]
	if second.Accepted != 1 || second.Parsed != 0 {
		t.Fatalf("input-b = %+v，期望 1/0", second)
	}
}
