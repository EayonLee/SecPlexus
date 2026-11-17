package iamclient

// PrincipalKey 是会话主体在请求上下文里的键。
const PrincipalKey = "iam.principal"

// AuditOutcome 是审计结果的闭集，与 IAM 侧枚举保持一致。
type AuditOutcome string

const (
	AuditOutcomeSuccess AuditOutcome = "success"
	AuditOutcomeDenied  AuditOutcome = "denied"
	AuditOutcomeFailure AuditOutcome = "failure"
)
