package parser_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
)

// corpusVariant 是一种"模式 + 参数"组合，对应一个语料文件。
// 语料按变体组织而不是按模式：同一种线格式可能有多套分隔符参数，
// 每套都要有自己的回归护栏（KV 的默认分隔符与冒号逗号就是两种）。
type corpusVariant struct {
	name string
	file string
	spec parser.Spec
}

var corpusVariants = []corpusVariant{
	{name: "json", file: "json.log", spec: parser.Spec{Mode: parser.ModeJSON}},
	{name: "cef", file: "cef.log", spec: parser.Spec{Mode: parser.ModeCEF}},
	{name: "kv", file: "kv.log", spec: parser.Spec{Mode: parser.ModeKV}},
	{
		name: "kv-colon",
		file: "kv-colon.log",
		spec: parser.Spec{Mode: parser.ModeKV, KV: parser.KVSpec{Delimiter: ":", Entry: ","}},
	},
	{
		name: "regex",
		file: "regex.log",
		spec: parser.Spec{Mode: parser.ModeRegex, Pattern: `(?P<time>\S+) (?P<level>\S+) (?P<msg>.*)`},
	},
	{
		name: "delimiter-comma",
		file: "delimiter.log",
		spec: parser.Spec{Mode: parser.ModeDelimiter, Delimiter: parser.DelimiterSpec{
			Separator: ",", Columns: []string{"a", "b", "c", "d"},
		}},
	},
}

// TestCorpusParsesEverySample 让每条语料过一遍真实的接收流程：
// 先按模式拆分（HTTP 批量推送与 NDJSON 的必经步骤），再逐条解析。
// 语料库是形态回归的护栏：某类真实写法解析不出来，这里立刻失败。
func TestCorpusParsesEverySample(t *testing.T) {
	for _, variant := range corpusVariants {
		t.Run(variant.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", "corpus", variant.file))
			if err != nil {
				t.Skipf("缺少语料 %s: %v", variant.file, err)
			}

			instance, err := parser.New(variant.spec)
			if err != nil {
				t.Fatalf("构造解析器失败: %v", err)
			}

			for index, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}

				// 走真实流程：先拆分成独立消息，再逐条解析。
				messages, err := parser.Split(variant.spec.Mode, []byte(line))
				if err != nil {
					t.Fatalf("第 %d 行拆分失败: %v\n样本: %s", index+1, err, line)
				}
				if len(messages) == 0 {
					t.Fatalf("第 %d 行拆出零条消息: %s", index+1, line)
				}

				for _, message := range messages {
					fields, err := instance.Parse(message)
					if err != nil {
						t.Fatalf("第 %d 行解析失败: %v\n样本: %s", index+1, err, line)
					}
					if len(fields) == 0 {
						t.Fatalf("第 %d 行解析出零个字段: %s", index+1, line)
					}
				}
			}
		})
	}
}

// 数组语料必须拆成两条：这是 HTTP 批量推送最常见的形态。
func TestCorpusArraySplitsPerElement(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "corpus", "json.log"))
	if err != nil {
		t.Skip(err)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[") {
			continue
		}

		messages, err := parser.Split(parser.ModeJSON, []byte(line))
		if err != nil {
			t.Fatalf("数组拆分失败: %v", err)
		}
		if len(messages) != 2 {
			t.Fatalf("拆出 %d 条，期望 2", len(messages))
		}
	}
}
