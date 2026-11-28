package internal_test

import (
	"io/fs"
	"os"
	"regexp"
	"strings"
	"testing"
)

// forbiddenIdentifiers 是改名后不应再出现在现行代码里的旧标识符。
//
// 这条测试存在的理由：改名靠人工 grep 收尾，下一次重构又会漏；
// 把"不该出现的词"变成会失败的测试，比写进文档有效。
var forbiddenIdentifiers = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{"旧表名 ingest.sources", regexp.MustCompile(`ingest\.sources\b`)},
	{"旧表名 parse_rules", regexp.MustCompile(`\bparse_rules\b`)},
	{"旧类型名 ParseRule", regexp.MustCompile(`\bParseRule\b`)},
	{"旧字段名 parse_status", regexp.MustCompile(`\bparse_status\b`)},
	{"旧类型名 normalizer.Rule", regexp.MustCompile(`normalizer\.Rule\b`)},
	{"旧通道名 TransportHTTP", regexp.MustCompile(`\bTransportHTTP\b`)},
	{"旧 API 路径 /api/v1/ingest/sources", regexp.MustCompile(`/api/v1/ingest/sources`)},
}

// 回归：现行代码里不能残留旧标识符。
func TestNoLegacyIdentifiersInIngestModule(t *testing.T) {
	scanned := 0
	module := os.DirFS("..")

	// 只扫描现行 Go 源码，版本化的 SQL 迁移保留历史名称。
	err := fs.WalkDir(module, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		content, err := fs.ReadFile(module, path)
		if err != nil {
			return err
		}

		scanned++

		for _, forbidden := range forbiddenIdentifiers {
			if forbidden.pattern.Match(content) {
				t.Errorf("%s 仍出现在 %s", forbidden.name, path)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// 扫描数为 0 说明路径写错了：宁可失败，也不要"零文件通过"的假绿灯。
	if scanned == 0 {
		t.Fatal("没有扫描到任何 Go 文件，测试路径可能失效")
	}
}
