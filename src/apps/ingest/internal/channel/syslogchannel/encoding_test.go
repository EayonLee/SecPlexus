package syslogchannel_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/syslogchannel"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

// 回归：GBK 的 syslog 报文必须在剥信封之前转码，否则主机名与消息都是乱码。
func TestGBKSyslogMessageIsDecoded(t *testing.T) {
	engine, err := normalizer.Compile(normalizer.Mapping{
		ID:        "gbk-syslog",
		Name:      "gbk-syslog",
		Parse:     parser.Spec{Mode: parser.ModeKV},
		FieldMap:  map[string]string{"message": "message", "syslog.hostname": "device.hostname"},
		Encoding:  "gbk",
		Constants: map[string]any{"class_uid": 2004},
	})
	if err != nil {
		t.Fatal(err)
	}

	port := freePort(t)
	registry := input.NewMemory()
	registry.Replace([]input.Entry{{
		ID: "gbk-1", Protocol: input.ProtocolSyslog, Port: port, Mapping: engine,
	}})

	publisher := &collector{}
	line := pipeline.New(pipeline.Options{Workers: 1, BatchSize: 1, FlushInterval: 10 * time.Millisecond}, publisher)

	ctx, cancel := context.WithCancel(context.Background())
	lineDone := make(chan struct{})
	go func() { defer close(lineDone); _ = line.Start(ctx) }()

	channel := syslogchannel.New(registry, line, time.Second, "127.0.0.1")
	channelDone := make(chan struct{})
	go func() { defer close(channelDone); _ = channel.Start(ctx) }()

	defer func() {
		cancel()
		<-channelDone
		<-lineDone
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		connection, dialErr := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if dialErr == nil {
			_ = connection.Close()

			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	// 用 GBK 编码一条含中文的 RFC5424 报文。
	plain := ` <34>1 2026-11-15T10:00:00Z 北京防火墙 app - - - message=暴力破解`
	encoded, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(plain[1:]))
	if err != nil {
		t.Fatal(err)
	}

	connection, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()

	if _, err = connection.Write(append([]byte("<34>1 "), encoded...)); err != nil {
		t.Fatal(err)
	}

	for time.Now().Before(deadline) && publisher.count() == 0 {
		time.Sleep(10 * time.Millisecond)
	}

	_ = line.Flush(context.Background())

	events := publisher.all()

	if len(events) == 0 {
		t.Fatal("没有收到事件")
	}

	var body map[string]any
	if err = json.Unmarshal(events[0].Body, &body); err != nil {
		t.Fatal(err)
	}

	message, _ := body["message"].(string)
	if message != "message=\u66b4\u529b\u7834\u89e3" && message != "暴力破解" && !containsChinese(message) {
		t.Fatalf("中文没有解码: %q", message)
	}
}

// containsChinese 判断字符串里是否含中文字符。
func containsChinese(value string) bool {
	for _, r := range value {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}

	return false
}
