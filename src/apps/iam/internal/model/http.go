// Package model 定义 IAM 模块自有的用例输入和 HTTP 投影。
package model

import (
	"github.com/aarondl/opt/omit"

	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

// APIPermissions 表示一次授权配置中的权限键集合。
type APIPermissions = []string

// APIRoleIDs 表示一次用户授权中的角色标识集合。
type APIRoleIDs = []string

// APIChangePasswordInput 绑定当前账号修改密码请求。
type APIChangePasswordInput struct {
	NewPassword string `json:"new_password" binding:"required,min=12,max=128"`
	OldPassword string `json:"old_password" binding:"required"`
}

// APIMe 是 /me 的响应：当前账号、直接持有的角色、有效权限与展示时区偏好。
type APIMe struct {
	Permissions []string            `json:"permissions"`        // 有效权限键，来自角色并集。
	Principal   iamclient.Principal `json:"principal"`          // 已验证的登录用户与版本。
	Roles       []APIRoleRef        `json:"roles"`              // 直接持有的角色，不含继承。
	TimeZone    string              `json:"timezone,omitempty"` // 展示时区偏好；空表示跟随浏览器。
}

// APIProfileUpdate 绑定本人资料更新；时区留空表示清除偏好。
// 指针用于区分“未提交该字段”和“显式清空”，未提交时保留原值。
type APIProfileUpdate struct {
	TimeZone *string `json:"timezone,omitempty" binding:"omitempty,max=64"`
}

// APIProfileUpdateResponse 返回保存后的展示时区偏好；空表示跟随浏览器。
type APIProfileUpdateResponse struct {
	TimeZone string `json:"timezone"`
}

// APIGrantsInput 绑定角色权限的全量替换。
type APIGrantsInput struct {
	Permissions APIPermissions `json:"permissions" binding:"iam_permissions"`
}

// APILoginInput 绑定用户名和密码，仅供凭证用例使用。
type APILoginInput struct {
	Password string `json:"password" binding:"required,max=128"`
	Username string `json:"username" binding:"required"`
}

// APILoginResponse 返回认证后的公开账号信息。
type APILoginResponse struct {
	Principal iamclient.Principal `json:"principal"`
}

// APIReauthenticateInput 绑定当前密码确认请求。
type APIReauthenticateInput struct {
	Password string `json:"password" binding:"required"`
}

// APIResetPasswordInput 绑定管理员重置密码请求。
type APIResetPasswordInput struct {
	Password string `json:"password" binding:"required,min=12,max=128"`
}

// APIRoleRef 是角色在用户投影中的最小引用。
type APIRoleRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// APIRole 是角色的公开投影，包含是否内置与权限键。
type APIRole struct {
	Builtin     bool     `json:"builtin"`
	Description string   `json:"description"`
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
	UserCount   int64    `json:"user_count"`
}

// APIRoleCreate 绑定角色创建：名称、说明与初始权限在创建时一次提交。
type APIRoleCreate struct {
	Description omit.Val[string] `json:"description,omitempty,omitzero" binding:"omitempty,max=1000"`
	Name        string           `json:"name" binding:"required,min=1,max=100"`
	Permissions APIPermissions   `json:"permissions" binding:"iam_permissions"`
}

// APIRoleInput 绑定角色更新；描述缺省保留、空值清空、null 拒绝。
type APIRoleInput struct {
	Description omit.Val[string] `json:"description,omitempty,omitzero" binding:"omitempty,max=1000"`
	Name        string           `json:"name" binding:"required,min=1,max=100"`
}

// APIUser 是账号公开投影，不含密码凭据；角色让列表与服务调用方看到授权来源。
type APIUser struct {
	AuthEpoch   int64        `json:"auth_epoch,string"`
	Configured  bool         `json:"configured"`
	DisplayName string       `json:"display_name"`
	Email       *string      `json:"email"`
	ID          string       `json:"id"`
	Roles       []APIRoleRef `json:"roles"`
	Status      string       `json:"status"`
	Username    string       `json:"username"`
}

// APIUserInput 绑定账号创建请求。角色可选：新账号可以先无角色，之后再分配。
type APIUserInput struct {
	DisplayName string   `json:"display_name" binding:"required,min=1,max=100"`
	Email       string   `json:"email,omitempty" binding:"omitempty"`
	Password    string   `json:"password" binding:"required,min=12,max=128"`
	RoleIDs     []string `json:"role_ids,omitempty" binding:"omitempty,dive,uuid"`
	Username    string   `json:"username" binding:"required"`
}

// APIUserUpdate 绑定账号更新；邮箱缺省保留、空值清空、null 拒绝。
// 会话失效由服务端在状态变更时推进 auth_epoch，不由客户端回传。
type APIUserUpdate struct {
	DisplayName string           `json:"display_name" binding:"required,min=1,max=100"`
	Email       omit.Val[string] `json:"email,omitempty,omitzero" binding:"omitempty"`
	Status      string           `json:"status" binding:"required,oneof=active disabled"`
}

// APIUserRolesInput 绑定用户角色全量替换。
type APIUserRolesInput struct {
	RoleIDs APIRoleIDs `json:"role_ids" binding:"iam_role_ids"`
}

// ListQuery 绑定分页与搜索条件。
type ListQuery struct {
	Limit  int    `form:"limit,default=10" binding:"min=1,max=100"`
	Offset int    `form:"offset,default=0" binding:"min=0,max=100000"`
	Q      string `form:"q"`
}

// AuditQuery 绑定审计列表的筛选与分页条件。
// 审计只支持向前游标：行数随时间单调增长，OFFSET 深翻会线性变慢。
// 可见范围由服务端的 iam.audit.read 决定，客户端不能指定额外范围。
type AuditQuery struct {
	Limit   int    `form:"limit,default=20" binding:"min=1,max=100"`
	Cursor  string `form:"cursor" binding:"omitempty,max=128"`
	Actor   string `form:"actor" binding:"omitempty,max=100"`
	Outcome string `form:"outcome" binding:"omitempty,oneof=success failure denied"`
	From    string `form:"from" binding:"omitempty,datetime=2006-01-02"`
	To      string `form:"to" binding:"omitempty,datetime=2006-01-02"`
}
