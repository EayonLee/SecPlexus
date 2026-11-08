package service

import "testing"

// 邮件是账号联系方式，非法值必须在业务层拒绝而不是写进数据库。
func TestValidEmail(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "empty means unset", value: "", valid: true},
		{name: "plain address", value: "analyst@example.com", valid: true},
		{name: "address with plus tag", value: "analyst+soc@example.com", valid: true},
		{name: "address with subdomain", value: "a.b@mail.example.co", valid: true},
		{name: "missing at sign", value: "analyst.example.com", valid: false},
		{name: "missing local part", value: "@example.com", valid: false},
		{name: "missing domain", value: "analyst@", valid: false},
		{name: "display name form", value: "Analyst <analyst@example.com>", valid: false},
		{name: "surrounding whitespace", value: " analyst@example.com", valid: false},
		{name: "two addresses", value: "a@example.com, b@example.com", valid: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := validEmail(tc.value); got != tc.valid {
				t.Fatalf("validEmail(%q) = %v, want %v", tc.value, got, tc.valid)
			}
		})
	}
}

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
