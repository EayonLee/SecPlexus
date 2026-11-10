// Package enum 定义 IAM 数据库中封闭取值的唯一来源。
//
// 每一组常量对应一列的 CHECK 约束：写库、比较和判断都使用这些常量，
// Go 代码不重复字符串字面量。新增或改名取值时只改本包与对应迁移，
// 编译器会指出所有需要同步比较的位置。
//
// 取值本身与 DDL 完全一致，见 docs/development/database-design.md。
package enum

// UserStatus 是 iam.users.status 的取值：账号能否登录。
type UserStatus string

const (
	// UserStatusActive 表示账号可正常登录。
	UserStatusActive UserStatus = "active"
	// UserStatusDisabled 表示账号被停用，所有会话立即失效。
	UserStatusDisabled UserStatus = "disabled"
)

// PermissionStatus 是 iam.permissions.status 的取值：权限是否仍在代码目录中。
type PermissionStatus string

const (
	// PermissionStatusActive 表示权限仍在目录中，可继续授权。
	PermissionStatusActive PermissionStatus = "active"
	// PermissionStatusRetired 表示权限已从目录移除，保留历史授权但立即失效。
	PermissionStatusRetired PermissionStatus = "retired"
)

// RiskLevel 是 iam.permissions.risk_level 的取值：动作的风险级别。
type RiskLevel string

const (
	// RiskLevelNormal 表示常规操作。
	RiskLevelNormal RiskLevel = "normal"
	// RiskLevelSensitive 表示会影响他人数据或授权的敏感操作。
	RiskLevelSensitive RiskLevel = "sensitive"
	// RiskLevelHigh 表示高风险操作，通常要求近期密码确认。
	RiskLevelHigh RiskLevel = "high"
)

// SessionState 是 iam.user_sessions.state 的取值：会话是否仍然有效。
type SessionState string

const (
	// SessionStateActive 表示会话有效且未过期。
	SessionStateActive SessionState = "active"
	// SessionStateRevoked 表示会话已被撤销，不能再次激活。
	SessionStateRevoked SessionState = "revoked"
)

// ActorType 是 iam.audit_events.actor_type 的取值：谁发起了这次操作。
type ActorType string

const (
	// ActorTypeUser 表示登录用户通过 HTTP 发起的操作。
	ActorTypeUser ActorType = "user"
	// ActorTypeSystem 表示系统自身发起的操作，例如登录失败记录。
	ActorTypeSystem ActorType = "system"
	// ActorTypeDeploymentOperator 表示运维命令发起的操作，例如恢复或重置密码。
	ActorTypeDeploymentOperator ActorType = "deployment_operator"
)

// AuditOutcome 是 iam.audit_events.outcome 的取值：操作结果。
type AuditOutcome string

const (
	// AuditOutcomeSuccess 表示操作已成功提交。
	AuditOutcomeSuccess AuditOutcome = "success"
	// AuditOutcomeDenied 表示因授权或凭据不足被拒绝。
	AuditOutcomeDenied AuditOutcome = "denied"
	// AuditOutcomeFailure 表示操作执行失败。
	AuditOutcomeFailure AuditOutcome = "failure"
)
