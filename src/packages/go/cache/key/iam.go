// Package key 定义平台实际使用的 Redis key 模板。
package key

import "fmt"

// Template 表示固定 spx 前缀的缓存 key 模板。
type Template string

const (
	IAMLoginIP      Template = "spx:iam:ratelimit:ip:%s"      // 按来源地址限制登录。
	IAMLoginAccount Template = "spx:iam:ratelimit:account:%s" // 按账号限制登录。
)

// Format 将已摘要的业务标识代入模板，不接受可配置前缀。
func (t Template) Format(identifier string) string { return fmt.Sprintf(string(t), identifier) }
