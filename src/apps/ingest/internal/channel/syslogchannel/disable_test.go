package syslogchannel_test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/syslogchannel"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

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
