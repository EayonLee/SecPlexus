package parser_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
)

// FuzzParseEveryMode 对五种解析模式共用的性质做模糊测试：
// 无论输入多恶劣（任意字节、超长、半截 UTF-8、嵌套极深），
// 解析器要么返回字段、要么返回错误——绝不能 panic，也不能返回零字段的"成功"。
//
// 种子来自语料库，因此变异是在真实写法的周边发生的，比随机字节更容易命中真实缺陷。
func FuzzParseEveryMode(f *testing.F) {
	seeds := []string{
		`{"src":"1.2.3.4"}`,
		`[{"src":"1.2.3.4"}]`,
		"CEF:0|V|P|1|1|n|3|src=1.2.3.4",
		"level=info msg=\"hi\"",
		"user:alice,action:login",
		"2026-11-14 high sql injection",
		"a,b,c,d",
		"",
		"\xff\xfe\x00",
	}

	// 语料库里的每一行都进种子。
	for _, variant := range corpusVariants {
		data, err := os.ReadFile(filepath.Join("testdata", "corpus", variant.file))
		if err != nil {
			continue
		}

		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				seeds = append(seeds, line)
			}
		}
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	instances := make(map[string]parser.Parser, len(corpusVariants))
	for _, variant := range corpusVariants {
		instance, err := parser.New(variant.spec)
		if err != nil {
			f.Fatalf("构造 %s 失败: %v", variant.name, err)
		}

		instances[variant.name] = instance
	}

	f.Fuzz(func(t *testing.T, input string) {
		for name, instance := range instances {
			fields, err := instance.Parse([]byte(input))
			if err != nil {
				continue
			}

			// 成功就必须有字段：零字段的"成功"会让下游产出空事件。
			if len(fields) == 0 {
				t.Fatalf("%s 对 %q 返回零字段且无错误", name, input)
			}
		}
	})
}

// FuzzSplitNeverPanics：拆分是接收路径的第一道闸，必须有界且不 panic。
func FuzzSplitNeverPanics(f *testing.F) {
	f.Add([]byte(`{"a":1}`))
	f.Add([]byte("[]"))
	f.Add([]byte("\n\n"))
	f.Add([]byte(strings.Repeat("[", 512)))

	f.Fuzz(func(t *testing.T, payload []byte) {
		for _, mode := range parser.Modes() {
			messages, err := parser.Split(mode, payload)
			if err != nil {
				continue
			}

			if len(messages) == 0 && !isEmptyJSONArray(mode, payload) {
				t.Fatalf("%s 拆分返回零条且无错误", mode)
			}
		}
	})
}

// isEmptyJSONArray 说明唯一的"成功但零条"情形：JSON 空数组。
//
// [] 是合法报文，语义是"本批没有事件"，因此不算错误；
// 显式建模而不是放宽断言，避免把真正的零输出缺陷一起放过。
func isEmptyJSONArray(mode string, payload []byte) bool {
	return mode == parser.ModeJSON && strings.TrimSpace(string(payload)) == "[]"
}
