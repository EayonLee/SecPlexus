// Package model 定义 Console 的公开协议。
package model

// Context 返回已授权的身份上下文。
type Context struct {
	DecisionID string `json:"decision_id"`
	UserID     string `json:"user_id"`
}
