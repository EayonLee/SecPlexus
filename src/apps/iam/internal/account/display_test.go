package account

import (
	"strings"
	"testing"
)

// TestValidateDisplayName 覆盖长度边界、trim 与多字节字符计数。
func TestValidateDisplayName(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		valid bool
		want  string
	}{
		{name: "single character", value: "a", valid: true, want: "a"},
		{name: "chinese name", value: "张三", valid: true, want: "张三"},
		{name: "100 runes", value: strings.Repeat("张", 100), valid: true, want: strings.Repeat("张", 100)},
		{name: "101 runes", value: strings.Repeat("张", 101), valid: false},
		{name: "empty", value: "", valid: false},
		{name: "spaces only", value: "   ", valid: false},
		{name: "tab only", value: "\t", valid: false},
		{name: "trimmed", value: "  张三  ", valid: true, want: "张三"},
		{name: "inner spaces kept", value: "张 三", valid: true, want: "张 三"},
		{name: "null byte rejected", value: "a\x00b", valid: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateDisplayName(tc.value)
			if (err == nil) != tc.valid {
				t.Fatalf("ValidateDisplayName(%q) error = %v, want valid %v", tc.value, err, tc.valid)
			}

			if tc.valid && got != tc.want {
				t.Fatalf("ValidateDisplayName(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}
