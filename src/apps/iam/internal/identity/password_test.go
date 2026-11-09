package identity

import (
	"strings"
	"testing"
)

// TestPasswordPolicy 覆盖普通密码的字符数、编码和最大长度边界。
func TestPasswordPolicy(t *testing.T) {
	for _, tc := range []struct {
		password string
		valid    bool
	}{
		{"12345678901", false},
		{"123456789012", true},
		{"admin", false},
		{"这是一个足够长的安全密码", true},
		{"a sufficiently long password", true},
		{strings.Repeat("a", 128), true},
		{strings.Repeat("a", 129), false},
		{"invalid\x00password", false},
		{string([]byte{0xff}), false},
	} {
		if err := ValidatePassword(tc.password); (err == nil) != tc.valid {
			t.Errorf("password policy: expected valid=%v", tc.valid)
		}
	}
}

// TestPasswordHash 验证哈希比对和参数资源限制。
func TestPasswordHash(t *testing.T) {
	h := PasswordHasher{}
	encoded, err := h.GenerateHash("123456789012")
	if err != nil {
		t.Fatal(err)
	}

	if err = h.CompareHashAndPassword(encoded, "123456789012"); err != nil {
		t.Fatal(err)
	}

	if err = h.CompareHashAndPassword(encoded, "another long password"); err == nil {
		t.Fatal("wrong password accepted")
	}

	if err = h.CompareHashAndPassword("$argon2id$v=19$m=999999999,t=1,p=1$AA$AA", "anything"); err == nil {
		t.Fatal("unbounded hash accepted")
	}
}
