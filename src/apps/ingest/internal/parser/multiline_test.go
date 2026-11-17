package parser_test

import (
	"strings"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
)

// 起始模式匹配新的日志行；堆栈续行并入上一条。
func TestMultilineMergesContinuationLines(t *testing.T) {
	merge, err := parser.NewMultiline(parser.Multiline{StartPattern: `^\d{4}-\d{2}-\d{2}`})
	if err != nil {
		t.Fatal(err)
	}

	payload := strings.Join([]string{
		"2026-11-15 10:00:00 ERROR boom",
		"java.lang.IllegalStateException: bad",
		"\tat com.example.Foo.bar(Foo.java:42)",
		"2026-11-15 10:00:01 INFO recovered",
	}, "\n")

	messages, err := merge([]byte(payload))
	if err != nil {
		t.Fatal(err)
	}

	if len(messages) != 2 {
		t.Fatalf("消息数 = %d，期望 2（堆栈应合并）", len(messages))
	}
	if !strings.Contains(string(messages[0]), "Foo.java:42") {
		t.Fatalf("堆栈没有并入第一条: %q", messages[0])
	}
	if !strings.Contains(string(messages[0]), "IllegalStateException") {
		t.Fatalf("续行丢失: %q", messages[0])
	}
}

// 首行即使不匹配起始模式也必须保留：丢弃会静默丢数据。
func TestMultilineKeepsLeadingLineEvenWithoutMatch(t *testing.T) {
	merge, err := parser.NewMultiline(parser.Multiline{StartPattern: `^START`})
	if err != nil {
		t.Fatal(err)
	}

	messages, err := merge([]byte("orphan line\nSTART real\nmore"))
	if err != nil {
		t.Fatal(err)
	}

	if len(messages) != 2 {
		t.Fatalf("消息数 = %d，期望 2", len(messages))
	}
	if string(messages[0]) != "orphan line" {
		t.Fatalf("首行被丢弃: %q", messages[0])
	}
}

// 行数上限必须生效，否则一条畸形输入就能吃掉内存。
func TestMultilineRespectsLineLimit(t *testing.T) {
	merge, err := parser.NewMultiline(parser.Multiline{StartPattern: `^START`, MaxLines: 3})
	if err != nil {
		t.Fatal(err)
	}

	payload := "START a\nb\nc\nd\ne"
	messages, err := merge([]byte(payload))
	if err != nil {
		t.Fatal(err)
	}

	if len(messages) < 2 {
		t.Fatalf("行数上限没生效，只得到 %d 条", len(messages))
	}
	for _, message := range messages {
		if strings.Count(string(message), "\n") >= 3 {
			t.Fatalf("单条消息超过 3 行: %q", message)
		}
	}
}

// 非法正则必须在构造期拒绝，不能等到运行期。
func TestMultilineRejectsInvalidPattern(t *testing.T) {
	if _, err := parser.NewMultiline(parser.Multiline{StartPattern: `(`}); err == nil {
		t.Fatal("非法正则被接受")
	}
	if _, err := parser.NewMultiline(parser.Multiline{}); err == nil {
		t.Fatal("空模式被接受")
	}
}
