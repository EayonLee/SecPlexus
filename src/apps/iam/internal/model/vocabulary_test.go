package model

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
)

// oneOf 从 binding 标签取出 oneof 允许值，供词表一致性断言使用。
func oneOf(t *testing.T, target any, field string) []string {
	t.Helper()
	structField, ok := reflect.TypeOf(target).FieldByName(field)
	if !ok {
		t.Fatalf("%T has no field %s", target, field)
	}
	for _, rule := range strings.Split(structField.Tag.Get("binding"), ",") {
		if values, found := strings.CutPrefix(rule, "oneof="); found {
			return strings.Fields(values)
		}
	}
	t.Fatalf("%T.%s has no oneof rule", target, field)
	return nil
}

// HTTP 绑定标签不能与数据库词表漂移：新增取值先改 enum，再同步标签。
// 标签是编译期字符串，无法直接引用常量，因此用本测试守住两处一致。
func TestBindingVocabularyMatchesEnums(t *testing.T) {
	for _, tc := range []struct {
		name   string
		actual []string
		want   []string
	}{
		{
			name:   "user status",
			actual: oneOf(t, APIUserUpdate{}, "Status"),
			want:   []string{string(enum.UserStatusActive), string(enum.UserStatusDisabled)},
		},
		{
			name:   "audit outcome",
			actual: oneOf(t, AuditQuery{}, "Outcome"),
			want:   []string{string(enum.AuditOutcomeSuccess), string(enum.AuditOutcomeFailure), string(enum.AuditOutcomeDenied)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := slices.Clone(tc.actual)
			want := slices.Clone(tc.want)
			slices.Sort(got)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatalf("binding allowed values %v; enum vocabulary %v", tc.actual, tc.want)
			}
		})
	}
}
