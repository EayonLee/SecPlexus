// Package authorization 拥有 IAM 权限业务目录、授权与委派规则。
package authorization

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
)

// BuiltinRoleName 是配置托管管理员自动持有的内置角色名称。
// 权限目录、内置角色与配置管理员都在启动时幂等写入数据库，前端只读。
const BuiltinRoleName = "超级管理员"

//go:embed permissions.json
var catalogJSON []byte

// Permission 定义稳定权限及其委派约束；权限是扁平的，没有作用域。
//
// 权限键本身就是唯一稳定标识，格式固定为 module.resource.action，
// 因此 Module/ResourceType/Action 不单独存储，避免两处定义漂移。
type Permission struct {
	Key                string                `json:"key" gorm:"column:permission_key"` // 稳定权限键；列名避开 SQL 保留字。
	Description        string                `json:"description"`                      // 公开用途说明。
	RiskLevel          enum.RiskLevel        `json:"risk_level"`                       // 动作风险级别，取值见 enum.RiskLevel。
	Delegable          bool                  `json:"delegable"`                        // 是否允许委派给自定义角色。
	RequiresRecentAuth bool                  `json:"requires_recent_auth"`             // 操作前是否需要近期密码确认。
	Status             enum.PermissionStatus `json:"status"`                           // 目录状态，取值见 enum.PermissionStatus。
	Module             string                `json:"module" gorm:"-"`                  // 由键派生的模块名，仅用于投影。
	ResourceType       string                `json:"resource_type" gorm:"-"`           // 由键派生的资源类型。
	Action             string                `json:"action" gorm:"-"`                  // 由键派生的操作。
}

// ParsePermissionKey 把 module.resource.action 拆成三段；任一段非法即视为目录错误。
func ParsePermissionKey(key string) (module, resource, action string, ok bool) {
	parts := strings.Split(key, ".")
	if len(parts) != 3 {
		return "", "", "", false
	}

	for _, part := range parts {
		if part == "" || strings.ToLower(part) != part {
			return "", "", "", false
		}
	}

	return parts[0], parts[1], parts[2], true
}

// Catalog 返回新的权限目录副本，并为每项填好派生的模块、资源与操作。
// 嵌入目录是代码资产：键不合法时在启动阶段就失败，而不是运行时悄无声息。
func Catalog() []Permission {
	var permissions []Permission
	if err := json.Unmarshal(catalogJSON, &permissions); err != nil {
		panic(fmt.Sprintf("invalid embedded permission catalog: %v", err))
	}

	seen := make(map[string]bool, len(permissions))
	for i := range permissions {
		p := &permissions[i]
		module, resource, action, ok := ParsePermissionKey(p.Key)
		if !ok {
			panic(fmt.Sprintf("permission key %q must be module.resource.action", p.Key))
		}

		if seen[p.Key] {
			panic(fmt.Sprintf("duplicate permission key %q", p.Key))
		}

		seen[p.Key] = true
		p.Module, p.ResourceType, p.Action = module, resource, action
	}

	return permissions
}
