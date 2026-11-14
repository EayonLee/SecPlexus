package syslogchannel

import (
	"bytes"
	"testing"
)

// RFC6587 八位组计数分帧：长度前缀决定报文边界。
func TestOctetCountingFraming(t *testing.T) {
	first := "<34>1 2026-11-14T10:00:00Z host app - - - hello"
	stream := []byte("" + itoa(len(first)) + " " + first + "5 world")

	frames, trailing := splitFrames(stream)
	if len(trailing) != 0 {
		t.Fatalf("unexpected trailing bytes: %q", trailing)
	}
	if len(frames) != 2 {
		t.Fatalf("frames = %d, want 2", len(frames))
	}
	if string(frames[0]) != first || string(frames[1]) != "world" {
		t.Fatalf("frames = %q", frames)
	}
}

// 非透明分帧：换行分隔，CRLF 也要处理。
func TestNonTransparentFraming(t *testing.T) {
	frames, trailing := splitFrames([]byte("line one\r\nline two\n"))
	if len(trailing) != 0 {
		t.Fatalf("trailing = %q", trailing)
	}
	if len(frames) != 2 {
		t.Fatalf("frames = %d, want 2", len(frames))
	}
	if string(frames[0]) != "line one" || string(frames[1]) != "line two" {
		t.Fatalf("frames = %q", frames)
	}
}

// 未收全的报文留在残留里，等下一次读取拼接。
func TestIncompleteFrameStaysPending(t *testing.T) {
	short := []byte("10 hello")

	frames, trailing := splitFrames(short)
	if len(frames) != 0 {
		t.Fatalf("incomplete frame was emitted: %q", frames)
	}
	if !bytes.Equal(trailing, short) {
		t.Fatalf("trailing = %q, want the whole buffer", trailing)
	}
}

// 非法长度前缀退回非透明分帧，避免整条流卡死。
func TestInvalidOctetCountFallsBackToLines(t *testing.T) {
	frames, _ := splitFrames([]byte("999999999999999999999999 hello\nsecond\n"))
	if len(frames) < 1 {
		t.Fatal("no frames parsed")
	}
}

// 超过 64 KiB 的报文截断并报告。
func TestTruncateReportsLimit(t *testing.T) {
	long := bytes.Repeat([]byte("x"), MaxMessageBytes+10)

	trimmed, truncated := truncate(long)
	if !truncated {
		t.Fatal("truncation not reported")
	}
	if len(trimmed) != MaxMessageBytes {
		t.Fatalf("length = %d, want %d", len(trimmed), MaxMessageBytes)
	}

	if _, truncated = truncate([]byte("short")); truncated {
		t.Fatal("short message reported as truncated")
	}
}

// itoa 避免测试引入 strconv 的额外分支。
func itoa(value int) string {
	if value == 0 {
		return "0"
	}

	digits := make([]byte, 0, 6)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}

	return string(digits)
}
