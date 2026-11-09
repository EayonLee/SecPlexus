package identity

import (
	"strings"
	"testing"
)

// TestNormalizeUsername 覆盖长度边界、首字符与连续特殊符；保留名单由 ValidateUsernameAvailable 负责。
func TestNormalizeUsername(t *testing.T) {
	for _, tc := range []struct {
		name     string
		username string
		valid    bool
		want     string
	}{
		{name: "two chars too short", username: "ab", valid: false},
		{name: "three chars minimum", username: "abc", valid: true, want: "abc"},
		{name: "39 chars maximum", username: "a" + strings.Repeat("b", 38), valid: true, want: "a" + strings.Repeat("b", 38)},
		{name: "40 chars too long", username: "a" + strings.Repeat("b", 39), valid: false},
		{name: "starts with digit", username: "1abc", valid: false},
		{name: "starts with underscore", username: "_abc", valid: false},
		{name: "starts with hyphen", username: "-abc", valid: false},
		{name: "starts with period", username: ".abc", valid: false},
		{name: "ends with hyphen", username: "abc-", valid: false},
		{name: "ends with period", username: "abc.", valid: false},
		{name: "single hyphen inside", username: "a-b", valid: true, want: "a-b"},
		{name: "single underscore inside", username: "a_b", valid: true, want: "a_b"},
		{name: "single period inside", username: "a.b", valid: true, want: "a.b"},
		{name: "consecutive hyphens", username: "a--b", valid: false},
		{name: "consecutive underscores", username: "a__b", valid: false},
		{name: "consecutive periods", username: "a..b", valid: false},
		{name: "mixed consecutive separators", username: "a.-b", valid: false},
		{name: "space inside", username: "a b", valid: false},
		{name: "unicode rejected", username: "用户abc", valid: false},
		{name: "at sign rejected", username: "a@b", valid: false},
		{name: "uppercase normalized", username: "AbC", valid: true, want: "abc"},
		{name: "surrounding whitespace trimmed", username: "  abc  ", valid: true, want: "abc"},
		{name: "empty", username: "", valid: false},
		{name: "whitespace only", username: "   ", valid: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeUsername(tc.username)
			if (err == nil) != tc.valid {
				t.Fatalf("NormalizeUsername(%q) error = %v, want valid %v", tc.username, err, tc.valid)
			}

			if tc.valid && got != tc.want {
				t.Fatalf("NormalizeUsername(%q) = %q, want %q", tc.username, got, tc.want)
			}
		})
	}
}

// TestValidateUsernameAvailable 覆盖创建期保留标识；登录路径不应受此限制。
func TestValidateUsernameAvailable(t *testing.T) {
	for _, name := range []string{"root", "system", "master", "support", "api", "null", "undefined"} {
		if err := ValidateUsernameAvailable(name); err == nil {
			t.Errorf("ValidateUsernameAvailable(%q) accepted a reserved name", name)
		}
	}

	for _, name := range []string{"admin", "administrator", "analyst", "zhangsan"} {
		if err := ValidateUsernameAvailable(name); err != nil {
			t.Errorf("ValidateUsernameAvailable(%q) = %v, want accepted", name, err)
		}
	}
}
