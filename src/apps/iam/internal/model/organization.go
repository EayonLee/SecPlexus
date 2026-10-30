package model

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// OrganizationName 是请求边界规范化后的组织展示名称，最多 30 个 Unicode 字符。
type OrganizationName string

// UnmarshalJSON 去除首尾空白，拒绝控制字符与超长名称，供组织创建及更新共用。
func (name *OrganizationName) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if strings.ContainsFunc(value, unicode.IsControl) {
		return errors.New("organization name contains control characters")
	}
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) < 1 || utf8.RuneCountInString(value) > 30 {
		return errors.New("organization name must contain 1–30 characters")
	}
	*name = OrganizationName(value)
	return nil
}

// OrganizationSlug 是创建后不变的 3–30 位组织标识。
type OrganizationSlug string

var organizationSlugPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,28}[a-z0-9]$`)

// UnmarshalJSON 仅允许小写字母、数字、下划线与连字符，首尾必须满足标识规则。
func (slug *OrganizationSlug) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if !organizationSlugPattern.MatchString(value) {
		return errors.New("invalid organization identifier")
	}
	*slug = OrganizationSlug(value)
	return nil
}
