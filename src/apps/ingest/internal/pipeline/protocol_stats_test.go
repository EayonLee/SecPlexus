package pipeline_test

import (
	"context"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

// 回归：分协议计数必须按接入协议分别累加，不能都算到同一个桶里。
func TestStatsCountPerProtocol(t *testing.T) {
	line, _ := start(t, pipeline.Options{Workers: 2, BatchSize: 4, FlushInterval: 10 * time.Millisecond})

	httpEntry := input.Entry{ID: "h1", Protocol: input.ProtocolHTTP, Mapping: jsonRule(t)}
	syslogEntry := input.Entry{ID: "s1", Protocol: input.ProtocolSyslog, Mapping: jsonRule(t)}

	if err := line.Submit(context.Background(), httpEntry, []byte(`{"src":"10.0.0.1"}`), time.Now(), pipeline.Meta{}); err != nil {
		t.Fatal(err)
	}
	if err := line.Submit(context.Background(), httpEntry, []byte(`not json`), time.Now(), pipeline.Meta{}); err != nil {
		t.Fatal(err)
	}
	if err := line.Submit(context.Background(), syslogEntry, []byte(`{"src":"10.0.0.2"}`), time.Now(), pipeline.Meta{}); err != nil {
		t.Fatal(err)
	}

	_ = line.Flush(context.Background())

	stats := line.Stats()

	http := stats.ByProtocol["http"]
	if http.Accepted != 2 {
		t.Fatalf("http accepted = %d，期望 2", http.Accepted)
	}
	if http.Parsed != 1 {
		t.Fatalf("http parsed = %d，期望 1（一条合法一条非法）", http.Parsed)
	}

	syslog := stats.ByProtocol["syslog"]
	if syslog.Accepted != 1 || syslog.Parsed != 1 {
		t.Fatalf("syslog = %+v，期望 1/1", syslog)
	}

	if stats.Accepted != 3 {
		t.Fatalf("全局 accepted = %d，期望 3", stats.Accepted)
	}
}
