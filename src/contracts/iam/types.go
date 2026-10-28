// Package iam 发布各服务共用的身份与授权接口契约。
package iam

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed permissions.json
var catalogJSON []byte

// Permission 定义稳定权限及其范围和委派约束。
type Permission struct {
	Key                string `json:"key"`                  // 稳定权限键。
	Module             string `json:"module"`               // 能力所属模块。
	ResourceType       string `json:"resource_type"`        // 权限对应的资源类型。
	Action             string `json:"action"`               // 资源上的操作。
	Scope              string `json:"scope"`                // 平台或组织范围。
	Description        string `json:"description"`          // 公开用途说明。
	RiskLevel          string `json:"risk_level"`           // 动作风险级别。
	Delegable          bool   `json:"delegable"`            // 是否允许委派给自定义角色。
	RequiresRecentAuth bool   `json:"requires_recent_auth"` // 操作前是否需要近期密码确认。
	Status             string `json:"status"`               // 当前有效状态。
}

// Catalog 返回新的权限切片，防止调用方修改进程共享契约。
func Catalog() []Permission {
	var permissions []Permission
	if err := json.Unmarshal(catalogJSON, &permissions); err != nil {
		panic(fmt.Sprintf("invalid embedded permission catalog: %v", err))
	}

	return permissions
}

// Principal 记录已验证的身份与会话版本，不包含认证凭据。
type Principal = APIPrincipal

// AuthorizedScope 记录授权后的平台或组织数据范围。
type AuthorizedScope = APIAuthorizedScope

// Resource 描述请求方提供的可选资源上下文。
type Resource = APIResource

// AuthorizeRequest 内部服务转交的原始请求及目标能力。
type AuthorizeRequest = APIAuthorizeRequest

// CapabilityDecision 内部授权结果，业务服务仍须执行实际资源范围约束。
type CapabilityDecision = APICapabilityDecision
