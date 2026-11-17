package httpchannel_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/httpchannel"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

type multiCollector struct {
	mu     sync.Mutex
	events []pipeline.Event
}

func (c *multiCollector) Publish(_ context.Context, events []pipeline.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.events = append(c.events, events...)

	return nil
}

// 回归：映射声明多行合并时，一段 Java 堆栈必须成为一条事件，而不是四条。
func TestMultilinePayloadBecomesSingleEvent(t *testing.T) {
	engine, err := normalizer.Compile(normalizer.Mapping{
		ID:    "java-stack",
		Name:  "java-stack",
		Parse: parser.Spec{Mode: parser.ModeRegex, Pattern: `(?s)^(?P<msg>.*)$`},
		FieldMap: map[string]string{
			"msg": "message",
		},
		Constants: map[string]any{"class_uid": 2004},
		Multiline: parser.Multiline{StartPattern: `^\d{4}-\d{2}-\d{2}`},
	})
	if err != nil {
		t.Fatal(err)
	}

	registry := input.NewMemory()
	registry.Replace([]input.Entry{{
		ID: "conn-java", Protocol: input.ProtocolHTTP, Mapping: engine,
		Token: input.DigestToken(testToken),
	}})

	publisher := &multiCollector{}
	line := pipeline.New(pipeline.Options{Workers: 1, BatchSize: 4, FlushInterval: 20 * time.Millisecond}, publisher)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = line.Start(ctx) }()

	channel := httpchannel.New(registry, &fakeLimiter{}, line)
	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/ingest/events/{input_id}", channel)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	payload := strings.Join([]string{
		"2026-11-15 10:00:00 ERROR boom",
		"java.lang.IllegalStateException: bad",
		"\tat com.example.Foo.bar(Foo.java:42)",
	}, "\n")

	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/ingest/events/conn-java", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set("Content-Type", "text/plain")

	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()

	_ = line.Flush(context.Background())

	publisher.mu.Lock()
	events := append([]pipeline.Event(nil), publisher.events...)
	publisher.mu.Unlock()

	if len(events) != 1 {
		t.Fatalf("事件数 = %d，期望 1（堆栈应合并）", len(events))
	}

	var body map[string]any
	if err = json.Unmarshal(events[0].Body, &body); err != nil {
		t.Fatal(err)
	}

	message, _ := body["message"].(string)
	if !strings.Contains(message, "Foo.java:42") {
		t.Fatalf("消息没有包含堆栈续行: %q", message)
	}
}
