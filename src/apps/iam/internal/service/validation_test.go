package service

import "testing"

// 只有空白的名称在界面上等于没有名称，必须在服务端拒绝。
func TestValidName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "plain text", value: "审计员", valid: true},
		{name: "single character", value: "a", valid: true},
		{name: "empty", value: "", valid: false},
		{name: "spaces only", value: "   ", valid: false},
		{name: "tab only", value: "\t", valid: false},
		{name: "leading whitespace with text", value: " 审计员 ", valid: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := validName(tc.value); got != tc.valid {
				t.Fatalf("validName(%q) = %v, want %v", tc.value, got, tc.valid)
			}
		})
	}
}
