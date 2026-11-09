package identity

import (
	"errors"
	"regexp"
	"strings"
)

// emailPattern 采用 WHATWG HTML 对 input type=email 的务实校验规则，
// 该规范明确标注为对 RFC 5322 的故意违反；完整 RFC 正则不可行。
var emailPattern = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

// 长度上限来自 RFC 5321：整址 254，本地部 64。
const (
	maxEmailLength    = 254
	maxEmailLocalPart = 64
)

// ValidateEmail 校验可选邮箱；空串表示未设置。
func ValidateEmail(email string) error {
	if email == "" {
		return nil
	}

	if len(email) > maxEmailLength {
		return errors.New("email must not exceed 254 characters")
	}

	at := strings.LastIndexByte(email, '@')
	if at <= 0 || at > maxEmailLocalPart {
		return errors.New("email local part must be 1–64 characters")
	}

	if !emailPattern.MatchString(email) {
		return errors.New("email is not a valid address")
	}

	return nil
}

// NormalizeEmail 返回用于唯一性比较的小写形式；空串保持为空。
//
// 只做小写归一：不剥离加号子地址（RFC 5233 合法），不剥离点号
// （该规则仅对 gmail 消费者账号成立，无法从外部判断域名归属）。
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
