package identity

import (
	"strings"
	"testing"
)

// TestValidateEmail 覆盖空值可选、HTML5 语法、本地部与总长边界。
func TestValidateEmail(t *testing.T) {
	local64 := strings.Repeat("a", 64)
	local65 := strings.Repeat("a", 65)

	for _, tc := range []struct {
		name  string
		email string
		valid bool
	}{
		{name: "empty means unset", email: "", valid: true},
		{name: "plain", email: "analyst@example.com", valid: true},
		{name: "plus tag", email: "analyst+soc@example.com", valid: true},
		{name: "subdomain", email: "a.b@mail.example.co", valid: true},
		{name: "local part 64", email: local64 + "@example.com", valid: true},
		{name: "local part 65", email: local65 + "@example.com", valid: false},
		{name: "missing at", email: "analyst.example.com", valid: false},
		{name: "missing local", email: "@example.com", valid: false},
		{name: "missing domain", email: "analyst@", valid: false},
		{name: "display name form", email: "Analyst <analyst@example.com>", valid: false},
		{name: "space inside", email: "ana lyst@example.com", valid: false},
		{name: "two addresses", email: "a@example.com, b@example.com", valid: false},
		{name: "trailing dot domain", email: "analyst@example.com.", valid: false},
		{name: "domain label too long", email: "a@" + strings.Repeat("b", 64) + ".com", valid: false},
		{name: "total length 254", email: local64 + "@" + strings.Repeat("d", 185) + ".com", valid: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateEmail(tc.email)
			if (err == nil) != tc.valid {
				t.Fatalf("ValidateEmail(%q) error = %v, want valid %v", tc.email, err, tc.valid)
			}
		})
	}
}

// TestNormalizeEmail 验证唯一性键为小写且保留原始地址语义。
func TestNormalizeEmail(t *testing.T) {
	if got := NormalizeEmail("  Analyst@Example.COM  "); got != "analyst@example.com" {
		t.Fatalf("NormalizeEmail = %q, want lowercased trimmed form", got)
	}

	if got := NormalizeEmail(""); got != "" {
		t.Fatalf("NormalizeEmail(empty) = %q, want empty", got)
	}

	// 加号子地址必须保留：剥离会误伤合法地址。
	if got := NormalizeEmail("a+soc@example.com"); got != "a+soc@example.com" {
		t.Fatalf("NormalizeEmail stripped subaddress: %q", got)
	}

	// 点号不得归一：该规则仅对 gmail 消费者账号成立，无法从外部判断。
	if got := NormalizeEmail("a.b@example.com"); got != "a.b@example.com" {
		t.Fatalf("NormalizeEmail stripped dots: %q", got)
	}
}
