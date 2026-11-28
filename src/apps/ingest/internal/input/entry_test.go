package input_test

import (
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
)

func TestExpiredSnapshotFencesAllReceiveChannels(t *testing.T) {
	registry := input.NewMemory()
	token := input.DigestToken("stale-token")
	registry.ReplaceUntil([]input.Entry{
		{ID: "http", Protocol: input.ProtocolHTTP, Token: token},
		{ID: "syslog", Protocol: input.ProtocolSyslog, Port: 1514},
		{ID: "kafka", Protocol: input.ProtocolKafka, Kafka: input.KafkaBinding{Brokers: []string{"broker"}, Topic: "logs"}},
	}, time.Now().Add(-time.Second))
	if registry.Loaded() || len(registry.SyslogBindings()) != 0 || len(registry.KafkaSources()) != 0 {
		t.Fatal("expired snapshot remained available")
	}
	if _, ok := registry.ByTokenDigest(token); ok {
		t.Fatal("expired credential remained active")
	}
	if _, ok := registry.ByID("syslog"); ok {
		t.Fatal("a previously bound listener can still resolve its expired source")
	}
}

// Replace 建立索引后，按标识与令牌都能查到。
func TestMemoryLookupByIDAndToken(t *testing.T) {
	registry := input.NewMemory()
	registry.Replace([]input.Entry{{
		ID:       "conn-1",
		Protocol: input.ProtocolHTTP,
		Token:    input.DigestToken("secret-token"),
	}})

	if entry, ok := registry.ByID("conn-1"); !ok || entry.ID != "conn-1" {
		t.Fatalf("ByID = %#v, %v", entry, ok)
	}
	if _, ok := registry.ByID("conn-2"); ok {
		t.Fatal("unknown id was found")
	}

	if entry, ok := registry.ByTokenDigest(input.DigestToken("secret-token")); !ok || entry.ID != "conn-1" {
		t.Fatalf("ByTokenDigest = %#v, %v", entry, ok)
	}
	if _, ok := registry.ByTokenDigest(input.DigestToken("wrong")); ok {
		t.Fatal("wrong token was accepted")
	}
}

// 整体替换会立刻让被删除接入点的令牌失效。
func TestReplaceDropsRemovedTokens(t *testing.T) {
	registry := input.NewMemory()
	registry.Replace([]input.Entry{{
		ID: "conn-1", Protocol: input.ProtocolHTTP, Token: input.DigestToken("old"),
	}})

	registry.Replace(nil)

	if _, ok := registry.ByID("conn-1"); ok {
		t.Fatal("removed connector is still addressable")
	}
	if _, ok := registry.ByTokenDigest(input.DigestToken("old")); ok {
		t.Fatal("revoked token is still valid")
	}
}

// 未配置令牌的接入点不进令牌索引，但按标识仍可查。
func TestConnectorWithoutTokenIsNotAddressableByToken(t *testing.T) {
	registry := input.NewMemory()
	registry.Replace([]input.Entry{{
		ID: "conn-syslog", Protocol: input.ProtocolSyslog, Port: 5514,
	}})

	if entry, ok := registry.ByID("conn-syslog"); !ok || entry.HasToken() {
		t.Fatalf("entry = %#v, %v", entry, ok)
	}
	if _, ok := registry.ByTokenDigest(input.DigestToken("")); ok {
		t.Fatal("empty digest matched a connector without a token")
	}
}

// SyslogBindings 只返回已绑定端口的 syslog 接入点。
func TestSyslogBindingsFiltersByChannelAndPort(t *testing.T) {
	registry := input.NewMemory()
	registry.Replace([]input.Entry{
		{ID: "http", Protocol: input.ProtocolHTTP, Token: input.DigestToken("t")},
		{ID: "syslog-bound", Protocol: input.ProtocolSyslog, Port: 5514},
		{ID: "syslog-unbound", Protocol: input.ProtocolSyslog},
		{ID: "kafka", Protocol: input.ProtocolKafka},
	})

	bindings := registry.SyslogBindings()
	if len(bindings) != 1 || bindings[0].ID != "syslog-bound" {
		t.Fatalf("bindings = %#v", bindings)
	}
}

// 摘要必须稳定：同一令牌每次得到同一摘要，不同令牌不同摘要。
func TestDigestTokenIsStable(t *testing.T) {
	first := input.DigestToken("token-a")
	if again := input.DigestToken("token-a"); again != first {
		t.Fatal("digest is not stable")
	}
	if other := input.DigestToken("token-b"); other == first {
		t.Fatal("different tokens share a digest")
	}
}
