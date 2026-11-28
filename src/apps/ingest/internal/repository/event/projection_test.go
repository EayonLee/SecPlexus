package event_test

import (
	"strings"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository/event"
)

func TestEventTableMigrationUsesVersionedColumns(t *testing.T) {
	sql, err := event.EventTableMigration()
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"event_time DateTime64(3, 'UTC') COMMENT 'time | 事件发生时间'",
		"src_ip Nullable(IPv6) COMMENT 'src_endpoint.ip | 源 IP'",
		"dst_port Nullable(UInt16) COMMENT 'dst_endpoint.port | 目的端口'",
		"ORDER BY (class_uid, event_time, event_id)",
		"PARTITION BY toYYYYMM(event_time)",
		"CONSTRAINT event_parse_outcome CHECK",
	} {
		if !strings.Contains(sql, expected) {
			t.Errorf("DDL 未应用投影或保留策略: %s", expected)
		}
	}
	if strings.Contains(sql, "TTL ") {
		t.Fatal("事件表不能自动清理历史事件")
	}
}
