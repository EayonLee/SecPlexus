package syslogchannel_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/syslogchannel"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
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

func (c *collector) all() []pipeline.Event {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]pipeline.Event(nil), c.events...)
}

func (c *collector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.events)
}

// syslogRule 映射 syslog 信封里的主机与消息。
func syslogRule(t *testing.T) *normalizer.Compiled {
	t.Helper()

	engine, err := normalizer.CompileDefinition([]byte(`{"contract_version":"v1","parse":{"mode":"kv"},"branches":[{"id":"network","name":"Network","class_uid":4001,"field_map":{"host":"src_endpoint.hostname"}}]}`))
	if err != nil {
		t.Fatal(err)
	}

	return engine
}

// portSeq 给本包分配互不相同的端口。
//
// 之前的做法是"绑 0 取系统端口再关掉"，但关闭与真正绑定之间有窗口，
// 并行跑的其他包可能抢走同一端口，导致通道绑定失败、测试超时。
// 改为在固定区间内顺序分配并当场验证可用性，避免这个竞争。
var portSeq atomic.Int32

// freePort 返回一个当前可绑定的端口。
func freePort(t *testing.T) int {
	t.Helper()

	if portSeq.Load() == 0 {
		// 起始点固定但区间靠上，避开常用服务端口；同包内顺序递增不会重复。
		portSeq.Store(43000)
	}

	for attempt := 0; attempt < 200; attempt++ {
		port := int(portSeq.Add(1))

		// 同时验证 TCP 与 UDP 都能绑：通道两者都要用。
		tcp, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			continue
		}
		_ = tcp.Close()

		udp, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			continue
		}
		_ = udp.Close()

		return port
	}

	t.Fatal("找不到可用端口")

	return 0
}

// harness 启动一个绑定端口的 syslog 通道。
func harness(t *testing.T, port int) (*collector, *pipeline.Pipeline, func()) {
	t.Helper()
	return harnessWithRule(t, port, syslogRule(t))
}

func harnessWithRule(t *testing.T, port int, rule *normalizer.Compiled) (*collector, *pipeline.Pipeline, func()) {
	t.Helper()

	registry := input.NewMemory()
	registry.Replace([]input.Entry{{
		ID:       "syslog-1",
		Protocol: input.ProtocolSyslog,
		Port:     port,
		Mapping:  rule,
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

	registry := input.NewMemory()
	registry.Replace([]input.Entry{{
		ID: "syslog-1", Protocol: input.ProtocolSyslog, Port: firstPort, Mapping: syslogRule(t),
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

	waitListening(t, firstPort, true)
	connection, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", firstPort))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()

	// 修改已有接入点，并追加第二个接入点。原 TCP 连接也应使用新的来源。
	registry.Replace([]input.Entry{
		{ID: "syslog-1", Protocol: input.ProtocolSyslog, Port: firstPort, Mapping: syslogRule(t), Vendor: "New vendor", Product: "Live edge"},
		{ID: "syslog-2", Protocol: input.ProtocolSyslog, Port: secondPort, Mapping: syslogRule(t)},
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
		t.Fatal("reload did not bind the new source port")
	}

	if _, err = connection.Write([]byte("<34>1 2026-11-14T10:00:00Z fw01 app - - - refreshed\n")); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for publisher.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	events := publisher.all()
	if len(events) != 1 {
		t.Fatalf("已有 TCP 连接收到 %d 条事件", len(events))
	}
	var body struct {
		Metadata struct {
			SPX struct{ Vendor, Product string }
		}
	}
	if err := json.Unmarshal(events[0].Body, &body); err != nil {
		t.Fatal(err)
	}
	if body.Metadata.SPX.Vendor != "New vendor" || body.Metadata.SPX.Product != "Live edge" {
		t.Fatalf("已有连接未采用新的来源配置: %+v", body.Metadata.SPX)
	}
}

// 回归（P0）：UDP 读缓冲被复用，报文只能靠 copy 才能安全入队。
// 连续快速发 40 条，每条 hostname 唯一；任何一条内容错乱都会让
// 信封解析失败（事件变成 error）或 hostname 不在预期集合里。
func TestUDPBurstKeepsMessagesIntact(t *testing.T) {
	port := freePort(t)
	publisher, line, cleanup := harness(t, port)
	defer cleanup()

	connection, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()

	const total = 40
	expected := make(map[string]struct{}, total)

	for index := 0; index < total; index++ {
		host := fmt.Sprintf("host-%02d-abcdefghijklmnopqrstuvwxyz0123456789", index)
		expected[host] = struct{}{}

		payload := fmt.Sprintf("<34>1 2026-11-14T10:00:00Z %s app - - - host=%s seq=%d", host, host, index)
		if _, err = connection.Write([]byte(payload)); err != nil {
			t.Fatal(err)
		}
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && publisher.count() < total {
		time.Sleep(5 * time.Millisecond)
	}

	_ = line.Flush(context.Background())
	events := publisher.all()
	if len(events) != total {
		t.Fatalf("收到 %d 条，期望 %d", len(events), total)
	}

	for index, event := range events {
		if event.Outcome.Status != pipeline.StatusOK {
			t.Fatalf("第 %d 条解析失败（内容已被覆盖）: %s", index, event.Outcome.Reason)
		}

		var body map[string]any
		if err = json.Unmarshal(event.Body, &body); err != nil {
			t.Fatalf("第 %d 条事件体非法: %v", index, err)
		}

		device, _ := body["src_endpoint"].(map[string]any)
		host, _ := device["hostname"].(string)
		if _, ok := expected[host]; !ok {
			t.Fatalf("第 %d 条 hostname = %q，不在发送集合内", index, host)
		}
	}
}

func TestSyslogCEFUsesTheMatchedBranch(t *testing.T) {
	rule, err := normalizer.CompileDefinition([]byte(`{
		"contract_version":"v1","parse":{"mode":"cef"},
		"branches":[{"id":"auth","name":"Authentication","class_uid":3002,
			"match":[{"field":"SignatureID","equals":"32002"}],
			"field_map":{"duser":"user.name","dst":"dst_endpoint.ip"},
			"constants":{"activity_id":1,"status_id":2}}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	port := freePort(t)
	published, _, cleanup := harnessWithRule(t, port, rule)
	defer cleanup()
	connection, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()
	for _, log := range []string{
		"CEF:0|Fortinet|FortiGate|7|32002|Login failed|5|duser=operator dst=192.0.2.2",
		"CEF:0|Fortinet|FortiGate|7|99000|Other event|4|password=private-device-marker",
	} {
		frame := "<34>1 2026-11-26T10:00:00Z fw01 fortigate - - - " + log
		if _, err := connection.Write([]byte(frame)); err != nil {
			t.Fatal(err)
		}
	}
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()
	for published.count() < 2 {
		select {
		case <-timeout.C:
			t.Fatal("Syslog did not publish both records")
		case <-poll.C:
		}
	}
	classes := make(map[int]pipeline.Status)
	for _, event := range published.all() {
		var evidence struct {
			Class int `json:"class_uid"`
		}
		if err := json.Unmarshal(event.Body, &evidence); err != nil {
			t.Fatalf("invalid published event: %s, %v", event.Body, err)
		}
		classes[evidence.Class] = event.Outcome.Status
	}
	if classes[3002] != pipeline.StatusOK || classes[0] != pipeline.StatusError {
		t.Fatalf("unexpected Syslog classification: %v", classes)
	}
}
