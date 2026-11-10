package iamclient

import "time"

// Principal 是 IAM 认证后向受信服务返回的账号投影。
// 授权只由「用户 → 角色 → 权限」表达，因此身份里不再携带组织或超管标记。
type Principal struct {
	AuthEpoch         int64      `json:"auth_epoch,string"` // 字符串编码避免浏览器整数精度丢失。
	AuthenticatedAt   time.Time  `json:"authenticated_at"`
	Configured        bool       `json:"configured"` // 是否为配置托管的内置管理员。
	DisplayName       string     `json:"display_name"`
	ReauthenticatedAt *time.Time `json:"reauthenticated_at,omitempty"`
	SessionID         string     `json:"session_id"`
	UserID            string     `json:"user_id"`
	Username          string     `json:"username"`
}

// CapabilityDecision 是单次服务授权判定及排障标识。
// 授权只有「用户 → 角色 → 权限」，没有作用域字段可填。
type CapabilityDecision struct {
	Allowed    bool      `json:"allowed"`
	DecisionID string    `json:"decision_id"`
	Principal  Principal `json:"principal"`
}

// AuthorizeRequest 传递原用户上下文；服务凭证不能替代用户会话。
// 授权目标只有 action：角色与权限都是全局的，没有组织范围可选。
type AuthorizeRequest struct {
	Action    string `json:"action" binding:"required"`
	Cookie    string `json:"cookie"`
	CSRFToken string `json:"csrf_token"`
	Method    string `json:"method" binding:"required"`
	Origin    string `json:"origin"`
	Path      string `json:"path" binding:"required"`
}
