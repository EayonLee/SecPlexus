package connector_test

import (
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connector"
)

// Replace 建立索引后，按标识与令牌都能查到。
func TestMemoryLookupByIDAndToken(t *testing.T) {
	registry := connector.NewMemory()
	registry.Replace([]connector.Snapshot{{
		ID:      "conn-1",
		Channel: connector.ChannelHTTP,
		Token:   connector.DigestToken("secret-token"),
	}})

	if snapshot, ok := registry.ByID("conn-1"); !ok || snapshot.ID != "conn-1" {
		t.Fatalf("ByID = %#v, %v", snapshot, ok)
	}
	if _, ok := registry.ByID("conn-2"); ok {
		t.Fatal("unknown id was found")
	}

	if snapshot, ok := registry.ByTokenDigest(connector.DigestToken("secret-token")); !ok || snapshot.ID != "conn-1" {
		t.Fatalf("ByTokenDigest = %#v, %v", snapshot, ok)
	}
	if _, ok := registry.ByTokenDigest(connector.DigestToken("wrong")); ok {
		t.Fatal("wrong token was accepted")
	}
}

// 整体替换会立刻让被删除接入点的令牌失效。
func TestReplaceDropsRemovedTokens(t *testing.T) {
	registry := connector.NewMemory()
	registry.Replace([]connector.Snapshot{{
		ID: "conn-1", Channel: connector.ChannelHTTP, Token: connector.DigestToken("old"),
	}})

	registry.Replace(nil)

	if _, ok := registry.ByID("conn-1"); ok {
		t.Fatal("removed connector is still addressable")
	}
	if _, ok := registry.ByTokenDigest(connector.DigestToken("old")); ok {
		t.Fatal("revoked token is still valid")
	}
}

// 未配置令牌的接入点不进令牌索引，但按标识仍可查。
func TestConnectorWithoutTokenIsNotAddressableByToken(t *testing.T) {
	registry := connector.NewMemory()
	registry.Replace([]connector.Snapshot{{
		ID: "conn-syslog", Channel: connector.ChannelSyslog, Port: 5514,
	}})

	if snapshot, ok := registry.ByID("conn-syslog"); !ok || snapshot.HasToken() {
		t.Fatalf("snapshot = %#v, %v", snapshot, ok)
	}
	if _, ok := registry.ByTokenDigest(connector.DigestToken("")); ok {
		t.Fatal("empty digest matched a connector without a token")
	}
}

// SyslogBindings 只返回已绑定端口的 syslog 接入点。
func TestSyslogBindingsFiltersByChannelAndPort(t *testing.T) {
	registry := connector.NewMemory()
	registry.Replace([]connector.Snapshot{
		{ID: "http", Channel: connector.ChannelHTTP, Token: connector.DigestToken("t")},
		{ID: "syslog-bound", Channel: connector.ChannelSyslog, Port: 5514},
		{ID: "syslog-unbound", Channel: connector.ChannelSyslog},
		{ID: "kafka", Channel: connector.ChannelKafka},
	})

	bindings := registry.SyslogBindings()
	if len(bindings) != 1 || bindings[0].ID != "syslog-bound" {
		t.Fatalf("bindings = %#v", bindings)
	}
}

// 摘要必须稳定：同一令牌每次得到同一摘要，不同令牌不同摘要。
func TestDigestTokenIsStable(t *testing.T) {
	first := connector.DigestToken("token-a")
	if again := connector.DigestToken("token-a"); again != first {
		t.Fatal("digest is not stable")
	}
	if other := connector.DigestToken("token-b"); other == first {
		t.Fatal("different tokens share a digest")
	}
}
