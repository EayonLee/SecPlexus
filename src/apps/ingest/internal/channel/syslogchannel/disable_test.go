package syslogchannel_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/syslogchannel"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

func TestReloadClosesEstablishedTCPConnection(t *testing.T) {
	port := freePort(t)
	registry := input.NewMemory()
	registry.Replace([]input.Entry{{
		ID: "syslog-1", Protocol: input.ProtocolSyslog, Port: port, Mapping: syslogRule(t),
	}})
	publisher := &collector{}
	line := pipeline.New(pipeline.Options{Workers: 1, BatchSize: 1}, publisher)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- line.Start(ctx) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()

	channel := syslogchannel.New(registry, line, time.Hour, "127.0.0.1")
	if err := channel.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	defer channel.Close()

	dialer := net.Dialer{}
	connection, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()
	message := "<34>1 2026-11-14T10:00:00Z fw01 app - - - first message"
	if _, err = fmt.Fprintf(connection, "%d %s", len(message), message); err != nil {
		t.Fatal(err)
	}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for publisher.count() == 0 {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("停用前未收到 TCP 样本")
		}
	}

	registry.Replace(nil)
	if err = channel.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if err = connection.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, err = connection.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("停用后 TCP 连接应已关闭，读取结果: %v", err)
	}
}

// 停用一个接入点必须立即释放端口，不需要重启进程。
//
// 这是运维承诺的一部分：停用后该端口不该再被占用，
// 否则"先停旧的、再换端口起新的"会互相打架，而且端口泄漏后只能重启。
func TestReloadReleasesPortOfDisabledInput(t *testing.T) {
	port := freePort(t)

	registry := input.NewMemory()
	registry.Replace([]input.Entry{{
		ID: "syslog-1", Protocol: input.ProtocolSyslog, Port: port, Mapping: syslogRule(t),
	}})

	line := pipeline.New(pipeline.Options{Workers: 1, BatchSize: 1, FlushInterval: 10 * time.Millisecond}, &collector{})

	ctx, cancel := context.WithCancel(context.Background())

	lineDone := make(chan struct{})
	go func() { defer close(lineDone); _ = line.Start(ctx) }()

	channel := syslogchannel.New(registry, line, 30*time.Millisecond, "127.0.0.1")
	channelDone := make(chan struct{})
	go func() { defer close(channelDone); _ = channel.Start(ctx) }()

	defer func() {
		cancel()
		<-channelDone
		<-lineDone
	}()

	waitListening(t, port, true)

	// 停用：注册表里不再有它。
	registry.Replace(nil)

	waitListening(t, port, false)
}

// waitListening 轮询端口状态，直到与期望一致或超时。
func waitListening(t *testing.T, port int, want bool) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	address := fmt.Sprintf("127.0.0.1:%d", port)

	for time.Now().Before(deadline) {
		listener, err := net.Listen("tcp", address)
		listening := err != nil
		if listener != nil {
			_ = listener.Close()
		}

		if listening == want {
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	if want {
		t.Fatalf("端口 %d 未开始监听", port)
	}

	t.Fatalf("端口 %d 在停用后仍被占用", port)
}
