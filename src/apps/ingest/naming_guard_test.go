package ingest_test

import (
	"io/fs"
	"os"
	"path/filepath"
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

// allowedPaths 是允许保留旧名的文件。
//
// 迁移是版本化历史：改已应用的文件会让数据库状态与代码不一致。
// 00001/00002 定义旧名，00003 的职责就是把旧名改成新名，二者必须写出旧名。
var allowedPaths = []string{
	"migrations/00001_sources.sql",
	"migrations/00002_table_comments.sql",
	"migrations/00003_rename_to_inputs_and_mappings.sql",
}

// 回归：现行代码里不能残留旧标识符。
func TestNoLegacyIdentifiersInIngestModule(t *testing.T) {
	scanned := 0

	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		for _, allowed := range allowedPaths {
			if strings.HasSuffix(filepath.ToSlash(path), allowed) {
				return nil
			}
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		scanned++

		for _, forbidden := range forbiddenIdentifiers {
			if forbidden.pattern.Match(content) {
				t.Errorf("%s 仍出现在 %s", forbidden.name, filepath.ToSlash(path))
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
