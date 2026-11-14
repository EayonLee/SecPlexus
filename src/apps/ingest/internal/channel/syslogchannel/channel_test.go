package syslogchannel_test

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/syslogchannel"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connector"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

// collector 记录已发布事件。
type collector struct {
	mu     sync.Mutex
	events []pipeline.Event
}

func (c *collector) Publish(_ context.Context, events []pipeline.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.events = append(c.events, events...)
	return nil
}

func (c *collector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.events)
}

// syslogRule 映射 syslog 信封里的主机与消息。
func syslogRule(t *testing.T) *normalizer.Engine {
	t.Helper()

	engine, err := normalizer.Compile(normalizer.Rule{
		ID:    "syslog-test",
		Parse: parser.Spec{Mode: parser.ModeKV},
		FieldMap: map[string]string{
			"syslog.hostname": "device.hostname",
			"message":         "message",
		},
		Constants: map[string]any{"class_uid": 2004},
	})
	if err != nil {
		t.Fatal(err)
	}

	return engine
}

// freePort 取一个当前可用的端口。
func freePort(t *testing.T) int {
	t.Helper()

	listener, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	port := listener.LocalAddr().(*net.UDPAddr).Port
	_ = listener.Close()

	return port
}

// harness 启动一个绑定端口的 syslog 通道。
func harness(t *testing.T, port int) (*collector, *pipeline.Pipeline, func()) {
	t.Helper()

	registry := connector.NewMemory()
	registry.Replace([]connector.Snapshot{{
		ID:      "syslog-1",
		Channel: connector.ChannelSyslog,
		Port:    port,
		Engine:  syslogRule(t),
	}})

	publisher := &collector{}
	line := pipeline.New(pipeline.Options{Workers: 2, BatchSize: 1, FlushInterval: 10 * time.Millisecond}, publisher)

	ctx, cancel := context.WithCancel(context.Background())
	lineDone := make(chan struct{})
	go func() { defer close(lineDone); _ = line.Start(ctx) }()

	channel := syslogchannel.New(registry, line, time.Second, "127.0.0.1")
	channelDone := make(chan struct{})
	go func() { defer close(channelDone); _ = channel.Start(ctx) }()

	// 等监听器就位，避免测试比绑定更快。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		connection, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			_ = connection.Close()
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	cleanup := func() {
		cancel()
		<-channelDone
		<-lineDone
	}

	return publisher, line, cleanup
}

// UDP 报文被接收、剥信封并映射成事件。
func TestUDPDeliveryIsParsed(t *testing.T) {
	port := freePort(t)
	publisher, line, cleanup := harness(t, port)
	defer cleanup()

	connection, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()

	line5424 := `<34>1 2026-11-14T10:00:00Z fw01 sshd - - - login failed`
	if _, err = connection.Write([]byte(line5424)); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && publisher.count() == 0 {
		time.Sleep(10 * time.Millisecond)
	}

	_ = line.Flush(context.Background())
	if publisher.count() == 0 {
		t.Fatal("UDP message was not ingested")
	}
}

// TCP 长连接按八位组计数分帧，两条报文各自成事件。
func TestTCPOctetCountingDeliversBothFrames(t *testing.T) {
	port := freePort(t)
	publisher, line, cleanup := harness(t, port)
	defer cleanup()

	first := `<34>1 2026-11-14T10:00:00Z fw01 app - - - first message`
	second := `<34>1 2026-11-14T10:00:01Z fw02 app - - - second message`

	payload := fmt.Sprintf("%d %s%d %s", len(first), first, len(second), second)

	connection, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()

	if _, err = connection.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && publisher.count() < 2 {
		time.Sleep(10 * time.Millisecond)
	}

	_ = line.Flush(context.Background())
	if publisher.count() < 2 {
		t.Fatalf("ingested %d events, want 2", publisher.count())
	}
}

// 热重载：注册表新增接入点后，新端口开始接收。
func TestReloadBindsNewConnector(t *testing.T) {
	firstPort := freePort(t)
	secondPort := freePort(t)

	registry := connector.NewMemory()
	registry.Replace([]connector.Snapshot{{
		ID: "syslog-1", Channel: connector.ChannelSyslog, Port: firstPort, Engine: syslogRule(t),
	}})

	publisher := &collector{}
	line := pipeline.New(pipeline.Options{Workers: 1, BatchSize: 1, FlushInterval: 10 * time.Millisecond}, publisher)

	ctx, cancel := context.WithCancel(context.Background())

	lineDone := make(chan struct{})
	go func() { defer close(lineDone); _ = line.Start(ctx) }()

	channel := syslogchannel.New(registry, line, 50*time.Millisecond, "127.0.0.1")
	channelDone := make(chan struct{})
	go func() { defer close(channelDone); _ = channel.Start(ctx) }()

	// 顺序必须写在一个 defer 里：先取消再等待。
	// 分成两个 defer 会因 LIFO 变成"先等待后取消"，直接死锁。
	defer func() {
		cancel()
		<-channelDone
		<-lineDone
	}()

	// 追加一个接入点，周期刷新应当把它绑定。
	registry.Replace([]connector.Snapshot{
		{ID: "syslog-1", Channel: connector.ChannelSyslog, Port: firstPort, Engine: syslogRule(t)},
		{ID: "syslog-2", Channel: connector.ChannelSyslog, Port: secondPort, Engine: syslogRule(t)},
	})

	var bound bool
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		connection, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", secondPort))
		if err == nil {
			_ = connection.Close()
			bound = true
			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	if !bound {
		t.Fatal("reload did not bind the new connector port")
	}
}
