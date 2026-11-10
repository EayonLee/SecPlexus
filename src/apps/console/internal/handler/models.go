// Package handler 定义 Console 的公开协议。
package handler

// APIConsoleContext 返回已授权的身份上下文。
type APIConsoleContext struct {
	DecisionID string `json:"decision_id"`
	UserID     string `json:"user_id"`
}
