package identity

import (
	"errors"
	"regexp"
	"strings"
)

// usernamePattern 要求首尾均为字母数字，中间允许 . _ -；与数据库 CHECK 一致。
var usernamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{1,37}[A-Za-z0-9]$`)

// consecutiveSeparators 拒绝连续分隔符，避免 a--b / a..b 之类的歧义标识。
var consecutiveSeparators = regexp.MustCompile(`[._-]{2,}`)

// reservedUsernames 阻止新账号占用系统概念标识；比较使用小写形式。
//
// 保留名单只在创建账号时校验，不进入 NormalizeUsername：后者同时服务登录与
// 配置托管账号，若在其中拦截，会让已存在的账号无法登录。
var reservedUsernames = map[string]struct{}{
	"root":      {},
	"system":    {},
	"master":    {},
	"support":   {},
	"security":  {},
	"api":       {},
	"null":      {},
	"undefined": {},
}

// NormalizeUsername 校验用户名并返回规范化的小写形式。
func NormalizeUsername(username string) (string, error) {
	username = strings.TrimSpace(username)
	if !usernamePattern.MatchString(username) {
		return "", errors.New("username must be 3–39 ASCII letters, digits, underscores, periods or hyphens, starting with a letter")
	}

	if consecutiveSeparators.MatchString(username) {
		return "", errors.New("username cannot contain consecutive separators")
	}

	return strings.ToLower(username), nil
}

// ValidateUsernameAvailable 校验新账号名未占用保留标识；仅用于创建路径。
func ValidateUsernameAvailable(normalized string) error {
	if _, reserved := reservedUsernames[normalized]; reserved {
		return errors.New("username is reserved")
	}

	return nil
}
