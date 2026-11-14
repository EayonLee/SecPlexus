package pipeline_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connector"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

// recorder 记录发布批次，可按次数注入失败。
type recorder struct {
	mu        sync.Mutex
	batches   [][]pipeline.Event
	failFirst int
	calls     int
}

func (r *recorder) Publish(_ context.Context, events []pipeline.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls++
	if r.calls <= r.failFirst {
		return errors.New("broker unavailable")
	}

	r.batches = append(r.batches, append([]pipeline.Event(nil), events...))
	return nil
}

func (r *recorder) events() []pipeline.Event {
	r.mu.Lock()
	defer r.mu.Unlock()

	var all []pipeline.Event
	for _, batch := range r.batches {
		all = append(all, batch...)
	}

	return all
}

func jsonRule(t *testing.T) *normalizer.Engine {
	t.Helper()

	engine, err := normalizer.Compile(normalizer.Rule{
		ID:        "pipeline-test",
		Parse:     parser.Spec{Mode: parser.ModeJSON},
		FieldMap:  map[string]string{"src": "src_endpoint.ip"},
		Constants: map[string]any{"class_uid": 2004},
	})
	if err != nil {
		t.Fatal(err)
	}

	return engine
}

func start(t *testing.T, options pipeline.Options) (*pipeline.Pipeline, *recorder) {
	t.Helper()

	publisher := &recorder{}
	line := pipeline.New(options, publisher)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = line.Start(ctx) }()

	return line, publisher
}

func TestSubmitParsesIntoOcsfEvent(t *testing.T) {
	line, publisher := start(t, pipeline.Options{Workers: 2, BatchSize: 2, FlushInterval: 20 * time.Millisecond})
	snapshot := connector.Snapshot{ID: "conn-1", Channel: connector.ChannelHTTP, Engine: jsonRule(t)}

	if err := line.Submit(context.Background(), snapshot, []byte(`{"src":"10.0.0.7"}`), time.Now(), pipeline.Meta{}); err != nil {
		t.Fatal(err)
	}

	line.Flush(context.Background())

	events := publisher.events()
	if len(events) != 1 {
		t.Fatalf("published %d events, want 1", len(events))
	}

	event := events[0]
	if event.Outcome.Status != pipeline.StatusOK {
		t.Fatalf("status = %q", event.Outcome.Status)
	}
	if event.EventID == "" {
		t.Fatal("event id is empty")
	}
	if event.PartitionKey != "10.0.0.7" {
		t.Fatalf("partition key = %q", event.PartitionKey)
	}

	var body map[string]any
	if err := json.Unmarshal(event.Body, &body); err != nil {
		t.Fatalf("event body is not JSON: %v", err)
	}
	metadata, ok := body["metadata"].(map[string]any)
	if !ok || metadata["parse_status"] != "ok" {
		t.Fatalf("metadata = %#v", body["metadata"])
	}
	if metadata["connector_id"] != "conn-1" {
		t.Fatalf("connector id missing: %#v", metadata)
	}
}

func TestParseFailureStillPublishesWithStatus(t *testing.T) {
	line, publisher := start(t, pipeline.Options{Workers: 1, BatchSize: 1, FlushInterval: 20 * time.Millisecond})
	snapshot := connector.Snapshot{ID: "conn-1", Channel: connector.ChannelHTTP, Engine: jsonRule(t)}

	if err := line.Submit(context.Background(), snapshot, []byte(`this is not json`), time.Now(), pipeline.Meta{}); err != nil {
		t.Fatal(err)
	}

	line.Flush(context.Background())

	events := publisher.events()
	if len(events) != 1 {
		t.Fatalf("published %d events, want 1", len(events))
	}

	event := events[0]
	if event.Outcome.Status != pipeline.StatusError {
		t.Fatalf("status = %q, want error", event.Outcome.Status)
	}
	if event.Outcome.Reason == "" {
		t.Fatal("missing reason code")
	}

	var body map[string]any
	if err := json.Unmarshal(event.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["class_uid"] != float64(pipeline.ClassUIDGeneric) {
		t.Fatalf("fallback class = %#v", body["class_uid"])
	}
	if body["message"] != "this is not json" {
		t.Fatalf("raw message not preserved: %#v", body["message"])
	}
}

func TestGenericConnectorPassesThrough(t *testing.T) {
	line, publisher := start(t, pipeline.Options{Workers: 1, BatchSize: 1, FlushInterval: 20 * time.Millisecond})
	snapshot := connector.Snapshot{ID: "conn-generic", Channel: connector.ChannelHTTP}

	if err := line.Submit(context.Background(), snapshot, []byte("raw line"), time.Now(), pipeline.Meta{}); err != nil {
		t.Fatal(err)
	}

	line.Flush(context.Background())
	events := publisher.events()
	if len(events) != 1 || events[0].Outcome.Status != pipeline.StatusError {
		t.Fatalf("passthrough events = %#v", events)
	}
}

func TestPublishRetriesUntilSuccess(t *testing.T) {
	publisher := &recorder{failFirst: 2}
	line := pipeline.New(pipeline.Options{Workers: 1, BatchSize: 1, FlushInterval: 20 * time.Millisecond}, publisher)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = line.Start(ctx) }()

	snapshot := connector.Snapshot{ID: "conn-1", Channel: connector.ChannelHTTP, Engine: jsonRule(t)}
	if err := line.Submit(context.Background(), snapshot, []byte(`{"src":"10.0.0.1"}`), time.Now(), pipeline.Meta{}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(publisher.events()) == 1 {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	if len(publisher.events()) != 1 {
		t.Fatalf("event was not published after retries; calls = %d", publisher.calls)
	}
	if stats := line.Stats(); stats.PublishRetry < 2 {
		t.Fatalf("retry counter = %d, want at least 2", stats.PublishRetry)
	}
}

func TestStatsCountAcceptedParsedPublished(t *testing.T) {
	line, _ := start(t, pipeline.Options{Workers: 2, BatchSize: 2, FlushInterval: 20 * time.Millisecond})
	snapshot := connector.Snapshot{ID: "conn-1", Channel: connector.ChannelHTTP, Engine: jsonRule(t)}

	for index := 0; index < 5; index++ {
		if err := line.Submit(context.Background(), snapshot, []byte(`{"src":"10.0.0.1"}`), time.Now(), pipeline.Meta{}); err != nil {
			t.Fatal(err)
		}
	}

	line.Flush(context.Background())

	stats := line.Stats()
	if stats.Accepted != 5 || stats.Parsed != 5 || stats.Published != 5 {
		t.Fatalf("stats = %+v", stats)
	}
	if stats.ParseFailed != 0 {
		t.Fatalf("parse failures = %d, want 0", stats.ParseFailed)
	}
}

func TestSubmitBlocksWhenQueueIsFull(t *testing.T) {
	// 不启动 Start：队列容量 1 会被第一次提交填满。
	line := pipeline.New(pipeline.Options{QueueDepth: 1}, &recorder{})
	snapshot := connector.Snapshot{ID: "conn-1", Channel: connector.ChannelHTTP, Engine: jsonRule(t)}

	if err := line.Submit(context.Background(), snapshot, []byte(`{"src":"1.1.1.1"}`), time.Now(), pipeline.Meta{}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := line.Submit(ctx, snapshot, []byte(`{"src":"1.1.1.2"}`), time.Now(), pipeline.Meta{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("submit error = %v, want deadline exceeded", err)
	}
}

func TestRepeatedIdenticalPayloadGetsDistinctIDs(t *testing.T) {
	line, publisher := start(t, pipeline.Options{Workers: 1, BatchSize: 2, FlushInterval: 20 * time.Millisecond})
	snapshot := connector.Snapshot{ID: "conn-1", Channel: connector.ChannelHTTP, Engine: jsonRule(t)}

	payload := []byte(`{"src":"10.0.0.9"}`)
	for attempt := 0; attempt < 2; attempt++ {
		if err := line.Submit(context.Background(), snapshot, payload, time.Now(), pipeline.Meta{}); err != nil {
			t.Fatal(err)
		}
	}

	line.Flush(context.Background())

	events := publisher.events()
	if len(events) != 2 {
		t.Fatalf("published %d events, want 2", len(events))
	}
	if events[0].EventID == events[1].EventID {
		t.Fatal("identical payloads collapsed into one event id")
	}
}
