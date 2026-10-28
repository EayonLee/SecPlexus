// Package identity 定义账号规则与密码哈希边界。
package identity

import (
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/aarondl/authboss/v3"
	"github.com/alexedwards/argon2id"
)

var ErrPasswordMismatch = errors.New("current password does not match")

var usernamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_.-]{2,63}$`)

// NormalizeUsername 校验用户名并返回规范化的小写形式。
func NormalizeUsername(username string) (string, error) {
	username = strings.TrimSpace(username)
	if !usernamePattern.MatchString(username) {
		return "", errors.New("username must be 3–64 ASCII letters, digits, underscores, periods or hyphens, starting with a letter")
	}

	return strings.ToLower(username), nil
}

// ValidatePassword 校验密码长度和字符编码。
func ValidatePassword(password string) error {
	length := utf8.RuneCountInString(password)
	if !utf8.ValidString(password) || length < 8 || length > 128 || len(password) > 512 || strings.ContainsRune(password, 0) {
		return errors.New("password must contain 8–128 Unicode characters")
	}

	return nil
}

// PasswordHasher 使用受限的 PHC 参数实现认证库密码哈希接口。
type PasswordHasher struct{}

var _ authboss.Hasher = PasswordHasher{}

// GenerateHash 校验密码并生成 Argon2id 哈希。
func (PasswordHasher) GenerateHash(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}

	return argon2id.CreateHash(password, &argon2id.Params{
		Memory:      64 * 1024,
		Iterations:  3,
		Parallelism: 2,
		SaltLength:  16,
		KeyLength:   32,
	})
}

// CompareHashAndPassword 校验哈希参数并验证密码。
func (PasswordHasher) CompareHashAndPassword(encoded, password string) error {
	if len(password) > 512 || len(encoded) > 512 {
		return errors.New("invalid credentials")
	}

	params, _, _, err := argon2id.DecodeHash(encoded)
	if err != nil {
		return errors.New("invalid password hash")
	}

	if params.Memory < 8*1024 || params.Memory > 128*1024 || params.Iterations < 1 || params.Iterations > 6 || params.Parallelism < 1 || params.Parallelism > 4 || params.SaltLength < 16 || params.SaltLength > 64 || params.KeyLength < 16 || params.KeyLength > 64 {
		return errors.New("password hash parameters outside supported bounds")
	}

	match, err := argon2id.ComparePasswordAndHash(password, encoded)
	if err != nil || !match {
		return errors.New("invalid credentials")
	}

	return nil
}
