// Package authorization 拥有 IAM 权限业务目录、授权与委派规则。
package authorization

import (
	"encoding/json"
	"fmt"
	_ "embed"
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

// Catalog 返回新的权限切片，防止调用方修改嵌入的业务目录。
func Catalog() []Permission {
	var permissions []Permission
	if err := json.Unmarshal(catalogJSON, &permissions); err != nil {
		panic(fmt.Sprintf("invalid embedded permission catalog: %v", err))
	}

	return permissions
}
