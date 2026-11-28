package middleware

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// Guard 是业务方唯一需要接的鉴权与审计中间件。
//
// 它把三件事做在一起，业务代码里不再出现任何权限判断：
//  1. CSRF：非安全方法必须带 CSRF 头，且来源与配置一致；
//  2. 会话与授权：调 IAM 的 /authorize，401/403/429/503 分别映射；
//  3. 审计：状态变更与所有拒绝写审计，读取成功不写（避免淹没管理留痕）。
//
// 放在中间件包而不是协议客户端：共享库停在 net/http 层，
// 与 Web 框架的适配属于使用方的基础设施。
func Guard(client *iamclient.Client, action, targetType string) gin.HandlerFunc {
	return guard(client, action, targetType, false)
}

// GuardWithDurableAudit 将成功写入的审计事实交由业务事务持久化；拒绝与失败仍由守卫记录。
func GuardWithDurableAudit(client *iamclient.Client, action, targetType string) gin.HandlerFunc {
	return guard(client, action, targetType, true)
}

func guard(client *iamclient.Client, action, targetType string, durableAudit bool) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		request := ctx.Request

		if unsafeMethod(request.Method) {
			if origin := request.Header.Get("Origin"); client.WebOrigin() != "" && origin != client.WebOrigin() {
				logging.From(request.Context()).Warn("Origin rejected for state change", "event", "auth.origin_rejected", "action", action)
				result.Fail(ctx.Writer, result.CodePermissionDenied)
				ctx.Abort()

				return
			}

			if request.Header.Get("X-CSRF-Token") == "" {
				logging.From(request.Context()).Warn("CSRF token missing", "event", "auth.csrf_missing", "action", action)
				result.Fail(ctx.Writer, result.CodePermissionDenied)
				ctx.Abort()

				return
			}
		}

		decision, err := client.Authorize(request.Context(), request, action)
		if err != nil {
			var failure *iamclient.Error
			if errors.As(err, &failure) {
				switch failure.Status {
				case http.StatusUnauthorized:
					recordAudit(client, request.Context(), request, action, targetType, ctx.Param("id"), string(iamclient.AuditOutcomeDenied), "")
					result.Fail(ctx.Writer, result.CodeUnauthenticated)
					ctx.Abort()

					return
				case http.StatusForbidden:
					recordAudit(client, request.Context(), request, action, targetType, ctx.Param("id"), string(iamclient.AuditOutcomeDenied), "")
					result.Fail(ctx.Writer, result.CodePermissionDenied)
					ctx.Abort()

					return
				case http.StatusTooManyRequests:
					ctx.Writer.Header().Set("Retry-After", "60")
					result.Fail(ctx.Writer, result.CodeRateLimited)
					ctx.Abort()

					return
				}
			}

			logging.From(request.Context()).Error("IAM authorization unavailable",
				"event", "auth.unavailable", "action", action, "error", logging.Error(err))
			result.Fail(ctx.Writer, result.CodeServiceUnavailable)
			ctx.Abort()

			return
		}

		ctx.Set(iamclient.PrincipalKey, decision)
		if durableAudit {
			ctx.Request = request.WithContext(iamclient.ContextWithAudit(request.Context(), iamclient.AuditEvent{
				Action: action, TargetType: targetType, Outcome: string(iamclient.AuditOutcomeSuccess),
				RequestID: RequestID(request), DecisionID: decision.DecisionID,
			}))
		}
		ctx.Next()

		outcome := string(iamclient.AuditOutcomeSuccess)
		if ctx.Writer.Status() >= 400 {
			outcome = string(iamclient.AuditOutcomeFailure)
		}

		if (unsafeMethod(request.Method) && !durableAudit) || outcome != string(iamclient.AuditOutcomeSuccess) {
			recordAudit(client, request.Context(), request, action, targetType, ctx.Param("id"), outcome, decision.DecisionID)
		}
	}
}

// Session 只校验会话，不校验具体权限。
func Session(client *iamclient.Client) gin.HandlerFunc {
	return Guard(client, iamclient.PermissionIAMUsersRead, "user")
}

// unsafeMethod 判断是否需要 CSRF 防护。
func unsafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}

	return true
}

// recordAudit 写入一条审计事实；失败只记 Error，不影响已经完成的业务响应。
//
// 这是显式的 fail-open：与 Kubernetes admission webhook 的 failurePolicy=Ignore 同语义，
// 即审计通道故障不阻断业务。要强一致需要事务性投递，属独立设计。
func recordAudit(client *iamclient.Client, ctx context.Context, request *http.Request, action, targetType, targetID, outcome, decisionID string) {
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()

	err := client.RecordAudit(recordCtx, iamclient.AuditEvent{
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Outcome:    outcome,
		RequestID:  RequestID(request),
		DecisionID: decisionID,
	})
	if err != nil {
		logging.From(ctx).Error("Could not record audit event", "event", "audit.write_failed",
			"action", action, "error", logging.Error(err))
	}
}
