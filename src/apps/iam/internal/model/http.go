// Package model 定义 IAM 模块自有的用例输入和 HTTP 投影。
package model

import (
	"github.com/aarondl/opt/omit"

	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

// APIPermissions 表示一次授权配置中的权限键集合。
type APIPermissions = []string

// APIRoleIDs 表示一次成员授权中的角色标识集合。
type APIRoleIDs = []string

// APIChangePasswordInput 绑定当前账号修改密码请求。
type APIChangePasswordInput struct {
	NewPassword string `json:"new_password" binding:"required,min=8,max=128"`
	OldPassword string `json:"old_password" binding:"required"`
}

// APICurrentOrganizationUpdate 绑定当前组织名称及并发版本。
type APICurrentOrganizationUpdate struct {
	AuthzVersion int64  `json:"authz_version,string" binding:"required,min=1"`
	Name         string `json:"name" binding:"required,min=1,max=100"`
}

// APIGrantsInput 绑定角色权限替换及并发版本。
type APIGrantsInput struct {
	Permissions APIPermissions `json:"permissions" binding:"required,iam_permissions"`
	Version     int64          `json:"version,string" binding:"required,min=1"`
}

// APILoginInput 绑定用户名和密码，仅供凭证用例使用。
type APILoginInput struct {
	Password string `json:"password" binding:"required,max=128"`
	Username string `json:"username" binding:"required"`
}

// APILoginResponse 返回认证后的公开身份。
type APILoginResponse struct {
	Principal iamclient.Principal `json:"principal"`
}

// APIMemberInput 绑定新增账号或加入已有账号的成员请求。
type APIMemberInput struct {
	DisplayName string `json:"display_name,omitempty" binding:"omitempty,min=1,max=100"`
	Email       string `json:"email,omitempty" binding:"omitempty"`
	Password    string `json:"password,omitempty" binding:"omitempty,min=8,max=128"`
	UserID      string `json:"user_id,omitempty" binding:"omitempty"`
	Username    string `json:"username,omitempty" binding:"omitempty"`
}

// APIMemberRolesInput 绑定成员角色替换及组织并发版本。
type APIMemberRolesInput struct {
	AuthzVersion int64      `json:"authz_version,string" binding:"required,min=1"`
	RoleIDs      APIRoleIDs `json:"role_ids" binding:"required,iam_role_ids"`
}

// APIMemberUpdate 绑定成员状态及组织并发版本。
type APIMemberUpdate struct {
	AuthzVersion int64  `json:"authz_version,string" binding:"required,min=1"`
	Status       string `json:"status" binding:"required,oneof=active disabled"`
}

// APIOrganization 是组织的公开投影。
type APIOrganization struct {
	AuthzVersion int64  `json:"authz_version,string"`
	ID           string `json:"id"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	Status       string `json:"status,omitempty"`
}

// APIOrganizationInput 绑定创建组织及初始管理员请求。
type APIOrganizationInput struct {
	AdminUserID string `json:"admin_user_id" binding:"required"`
	Name        string `json:"name" binding:"required,min=1,max=100"`
	Slug        string `json:"slug" binding:"required"`
}

// APIOrganizationUpdate 绑定平台组织更新请求。
type APIOrganizationUpdate struct {
	AuthzVersion int64  `json:"authz_version,string" binding:"required,min=1"`
	Name         string `json:"name" binding:"required,min=1,max=100"`
	Status       string `json:"status" binding:"required,oneof=active suspended"`
}

// APIPreview 表示授权变更的只读影响。
type APIPreview struct {
	Added           []string `json:"added"`
	AffectedMembers int64    `json:"affected_members"`
	After           []string `json:"after"`
	Before          []string `json:"before"`
	Removed         []string `json:"removed"`
}

// APIPreviewInput 绑定角色权限或成员角色预览；两类目标互斥。
type APIPreviewInput struct {
	AuthzVersion int64           `json:"authz_version,omitempty,string" binding:"required_with=MembershipID,omitempty,min=1"`
	MembershipID string          `json:"membership_id,omitempty" binding:"omitempty,uuid"`
	Permissions  *APIPermissions `json:"permissions,omitempty" binding:"required_with=RoleID,omitempty,iam_permissions"`
	RoleID       string          `json:"role_id,omitempty" binding:"omitempty,uuid"`
	RoleIDs      *APIRoleIDs     `json:"role_ids,omitempty" binding:"required_with=MembershipID,omitempty,iam_role_ids"`
	Version      int64           `json:"version,omitempty,string" binding:"required_with=RoleID,omitempty,min=1"`
}

// APIReauthenticateInput 绑定当前密码确认请求。
type APIReauthenticateInput struct {
	Password string `json:"password" binding:"required"`
}

// APIResetPasswordInput 绑定管理员重置密码请求。
type APIResetPasswordInput struct {
	Password string `json:"password" binding:"required,min=8,max=128"`
}

// APIRole 是包含允许操作的角色投影。
type APIRole struct {
	AllowedActions []string `json:"allowed_actions"`
	Code           string   `json:"code"`
	Description    string   `json:"description"`
	ID             string   `json:"id"`
	Kind           string   `json:"kind"`
	MemberCount    int64    `json:"member_count"`
	Name           string   `json:"name"`
	Permissions    []string `json:"permissions"`
	Status         string   `json:"status"`
	Version        int64    `json:"version,string"`
}

// APIRoleCreate 绑定自定义角色创建；描述缺省不等同于显式清空。
type APIRoleCreate struct {
	Code        string           `json:"code" binding:"required"`
	Description omit.Val[string] `json:"description,omitempty,omitzero" binding:"omitempty,max=1000"`
	Name        string           `json:"name" binding:"required,min=1,max=100"`
	Status      string           `json:"status,omitempty" binding:"omitempty,oneof=active disabled"`
}

// APIRoleInput 绑定角色更新；描述缺省保留、空值清空、null 拒绝。
type APIRoleInput struct {
	Description omit.Val[string] `json:"description,omitempty,omitzero" binding:"omitempty,max=1000"`
	Name        string           `json:"name" binding:"required,min=1,max=100"`
	Status      string           `json:"status" binding:"required,oneof=active disabled"`
	Version     int64            `json:"version,string" binding:"required,min=1"`
}

// APISetSuperAdminInput 绑定超级管理员变更；指针区分 false 与未提供。
type APISetSuperAdminInput struct {
	Enabled *bool  `json:"enabled" binding:"required"`
	Reason  string `json:"reason" binding:"required,min=1,max=500"`
}

// APIUser 是平台账号公开投影，不含密码凭据。
type APIUser struct {
	AuthVersion  int64   `json:"auth_version,string"`
	DisplayName  string  `json:"display_name"`
	Email        *string `json:"email"`
	ID           string  `json:"id"`
	IsSuperAdmin bool    `json:"is_super_admin"`
	Status       string  `json:"status"`
	Username     string  `json:"username"`
}

// APIUserInput 绑定平台账号创建请求。
type APIUserInput struct {
	DisplayName string `json:"display_name" binding:"required,min=1,max=100"`
	Email       string `json:"email,omitempty" binding:"omitempty"`
	Password    string `json:"password" binding:"required,min=8,max=128"`
	Username    string `json:"username" binding:"required"`
}

// APIUserUpdate 绑定账号更新；邮箱缺省保留、空值清空、null 拒绝。
type APIUserUpdate struct {
	AuthVersion int64            `json:"auth_version,string" binding:"required,min=1"`
	DisplayName string           `json:"display_name" binding:"required,min=1,max=100"`
	Email       omit.Val[string] `json:"email,omitempty,omitzero" binding:"omitempty"`
	Status      string           `json:"status" binding:"required,oneof=active disabled"`
}

// ListQuery 绑定分页与搜索，偏移最大为十万。
type ListQuery struct {
	Limit  int    `form:"limit,default=50" binding:"min=1,max=100"`
	Offset int    `form:"offset,default=0" binding:"min=0,max=100000"`
	Q      string `form:"q"`
	OrgID  string `form:"org_id" binding:"omitempty,uuid"`
}

// OrganizationQuery 绑定可选组织范围。
type OrganizationQuery struct {
	OrgID string `form:"org_id" binding:"omitempty,uuid"`
}

// VersionQuery 绑定资源并发版本。
type VersionQuery struct {
	Version int64 `form:"version" binding:"required,min=1"`
}

// AuthorizationVersionQuery 绑定组织授权并发版本。
type AuthorizationVersionQuery struct {
	AuthzVersion int64 `form:"authz_version" binding:"required,min=1"`
}
