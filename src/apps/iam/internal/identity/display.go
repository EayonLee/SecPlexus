package identity

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// 显示名称长度按 Unicode 码点计数，与数据库 char_length 语义一致。
const (
	minDisplayNameLength = 1
	maxDisplayNameLength = 100
)

// ValidateDisplayName 校验并返回去除首尾空白后的显示名称。
func ValidateDisplayName(displayName string) (string, error) {
	trimmed := strings.TrimSpace(displayName)
	length := utf8.RuneCountInString(trimmed)
	if !utf8.ValidString(trimmed) || strings.ContainsRune(trimmed, 0) || length < minDisplayNameLength || length > maxDisplayNameLength {
		return "", errors.New("display name must contain 1–100 Unicode characters")
	}

	return trimmed, nil
}
