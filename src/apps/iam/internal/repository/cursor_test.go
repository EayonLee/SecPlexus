package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// 游标必须原样还原排序键；否则翻页会跳过或重复事件。
func TestAuditCursorRoundTrip(t *testing.T) {
	id := uuid.NewString()
	at := time.Date(2026, 11, 10, 3, 4, 5, 123456789, time.UTC)
	gotTime, gotID, err := decodeAuditCursor(encodeAuditCursor(at, id))
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if !gotTime.Equal(at) || gotID != id {
		t.Fatalf("round trip = (%s,%s), want (%s,%s)", gotTime, gotID, at, id)
	}
}

// 非本服务签发的游标一律按无效输入拒绝，不能变成 SQL 注入或静默全表扫描。
func TestAuditCursorRejectsMalformedValues(t *testing.T) {
	for _, value := range []string{
		"not-base64!!",
		"",
		"bm8tc2VwYXJhdG9y",
	} {
		if _, _, err := decodeAuditCursor(value); !errors.Is(err, ErrInvalid) {
			t.Fatalf("cursor %q returned %v, want ErrInvalid", value, err)
		}
	}

	badTime := encodeAuditCursor(time.Time{}, uuid.NewString())
	if _, _, err := decodeAuditCursor(badTime); err == nil {
		t.Fatal("zero time cursor was accepted")
	}
}
