package enum_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
)

// 对齐测试：TargetType 常量集合必须与迁移里的 CHECK 完全一致。
//
// 只改一边的后果是静默的：代码写出数据库不接受的值，或数据库允许了代码从不产生的值。
// 这条测试让两边任何一次单边改动都失败。
func TestTargetTypeMatchesDatabaseConstraint(t *testing.T) {
	migrationPath := filepath.Join("..", "..", "migrations", "00001_init.sql")

	raw, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("读取迁移失败: %v", err)
	}

	// 取 CHECK (...) IN (...) 里的取值列表。
	pattern := regexp.MustCompile(`audit_events_target_type_check\s+CHECK\s*\(target_type\s+IN\s*\(([^)]+)\)\)`)
	matches := pattern.FindStringSubmatch(string(raw))
	if len(matches) != 2 {
		t.Fatal("迁移里找不到 target_type 的 CHECK 约束")
	}

	var fromDatabase []string
	for _, value := range strings.Split(matches[1], ",") {
		if trimmed := strings.Trim(strings.TrimSpace(value), "'"); trimmed != "" {
			fromDatabase = append(fromDatabase, trimmed)
		}
	}

	fromCode := []string{
		string(enum.TargetTypeUser),
		string(enum.TargetTypeSession),
		string(enum.TargetTypeRole),
		string(enum.TargetTypePlatform),
		string(enum.TargetTypeInput),
		string(enum.TargetTypeMapping),
	}

	sort.Strings(fromDatabase)
	sort.Strings(fromCode)

	if strings.Join(fromDatabase, ",") != strings.Join(fromCode, ",") {
		t.Fatalf("枚举与 CHECK 不一致\n数据库: %v\n代码:   %v", fromDatabase, fromCode)
	}
}
