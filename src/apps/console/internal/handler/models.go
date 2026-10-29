// Package handler 定义 Console 的公开协议。
package handler

// APIConsoleContext 返回已授权的组织及身份上下文。
type APIConsoleContext struct {
	DecisionID string `json:"decision_id"`
	OrgID      string `json:"org_id"`
	UserID     string `json:"user_id"`
}
