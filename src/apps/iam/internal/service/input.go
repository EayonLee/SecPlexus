package service

import (
	"github.com/aarondl/opt/omit"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

// 用例输入只表达业务需要的字段与已解析的取值。
// HTTP 结构留在 handler/model：service 不依赖任何传输协议类型，
// 因此换一种入口（CLI、消息队列）不需要改动业务代码。

// UserCreate 是创建账号的输入；字段格式已在入口边界校验。
type UserCreate struct {
	Username    string   // 登录名原样，规范化由用例完成。
	DisplayName string   // 显示名称。
	Email       string   // 可选联系邮箱；空串表示未设置。
	Password    string   // 初始密码明文，仅在内存中用于计算哈希。
	RoleIDs     []string // 可选初始角色。
}

// UserUpdate 是更新账号的输入；Status 是已解析的账号状态。
type UserUpdate struct {
	DisplayName string           // 显示名称。
	Email       omit.Val[string] // 未提交时保留原值，空值表示清空。
	Status      enum.UserStatus  // 目标账号状态。
}

// UserRoles 是全量替换账号角色的输入。
type UserRoles struct {
	RoleIDs []string // 目标角色集合，空集合表示不持有任何角色。
}

// RoleCreate 是创建角色的输入。
type RoleCreate struct {
	Name        string           // 角色名称。
	Description omit.Val[string] // 可选说明；未提交时为空。
	Permissions []string         // 初始权限键集合。
}

// RoleUpdate 是更新角色资料的输入。
type RoleUpdate struct {
	Name        string           // 角色名称。
	Description omit.Val[string] // 未提交时保留原说明。
}

// RoleGrants 是全量替换角色权限的输入。
type RoleGrants struct {
	Permissions []string // 目标权限键集合。
}

// AuditFilter 是审计列表的筛选与分页输入。
// Cursor 为空表示从最新事件开始；Limit 由入口绑定限制上限。
type AuditFilter struct {
	Limit   int
	Cursor  string
	Actor   string
	Outcome string
	From    string
	To      string
}

// Identity 是当前账号的授权投影：账号、直接角色、有效权限与展示时区。
type Identity struct {
	Permissions []string
	Principal   iamclient.Principal
	Roles       []repository.RoleRef
	TimeZone    string
}
