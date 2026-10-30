package iamclient

import "time"

// Principal 是 IAM 认证后向受信服务返回的身份投影。
type Principal struct {
	AuthVersion       int64      `json:"auth_version,string"` // 字符串编码避免浏览器整数精度丢失。
	AuthenticatedAt   time.Time  `json:"authenticated_at"`
	DisplayName       string     `json:"display_name"`
	IsSuperAdmin      bool       `json:"is_super_admin"`
	ReauthenticatedAt *time.Time `json:"reauthenticated_at,omitempty"`
	SessionID         string     `json:"session_id"`
	UserID            string     `json:"user_id"`
	Username          string     `json:"username"`
}

// AuthorizedScope 标识授权的作用域；组织范围携带 OrgID。
type AuthorizedScope struct {
	Kind  string `json:"kind"`
	OrgID string `json:"org_id,omitempty"`
}

// CapabilityDecision 是单次服务授权判定及排障标识。
type CapabilityDecision struct {
	Allowed    bool            `json:"allowed"`
	DecisionID string          `json:"decision_id"`
	Principal  Principal       `json:"principal"`
	Scope      AuthorizedScope `json:"scope"`
}

// AuthorizeRequest 传递原用户上下文；服务凭证不能替代用户会话。
type AuthorizeRequest struct {
	Action    string    `json:"action" binding:"required"`
	Cookie    string    `json:"cookie"`
	CSRFToken string    `json:"csrf_token"`
	Method    string    `json:"method" binding:"required"`
	OrgID     string    `json:"org_id,omitempty"`
	Origin    string    `json:"origin"`
	Path      string    `json:"path" binding:"required"`
	Resource  *Resource `json:"resource,omitempty"`
}

// Resource 是业务模块提交授权判定的资源引用。
type Resource struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}
