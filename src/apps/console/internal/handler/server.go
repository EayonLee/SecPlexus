// Package handler 按 IAM 授权结果处理 Console 请求，不信任浏览器声明的身份。
package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// Server 使用 IAM 的授权结果处理 Console 请求。
type Server struct {
	// ReauthInterval 是事件流重新鉴权的间隔；0 表示 30 秒。
	ReauthInterval time.Duration
	IAM            *iamclient.Client // 用于验证会话与扁平授权的 IAM 客户端。
}

// Context 返回已经 IAM 验证的当前身份上下文。
func (s *Server) Context(c *gin.Context) {
	w := c.Writer
	decision, ok := principal(c)
	if !ok {
		result.Fail(w, result.CodeUnauthenticated)

		return
	}

	result.Success(w, APIConsoleContext{
		UserID:     decision.Principal.UserID,
		DecisionID: decision.DecisionID,
	})
}

// Events 订阅事件并定期重新鉴权，权限撤销后结束流。
func (s *Server) Events(c *gin.Context) {
	w := c.Writer
	decision, ok := principal(c)
	if !ok {
		result.Fail(w, result.CodeUnauthenticated)

		return
	}

	logger := logging.From(c.Request.Context()).With("user_id", decision.Principal.UserID, "decision_id", decision.DecisionID)
	started := time.Now()
	reason := "client_disconnected"
	defer func() {
		logger.Info("事件流结束", "event", "sse.closed", "reason", reason, "duration_ms", time.Since(started).Milliseconds())
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	c.SSEvent("authorized", map[string]string{"decision_id": decision.DecisionID})
	if c.IsAborted() {
		reason = "write_failed"
		return
	}
	c.Writer.Flush()
	logger.Info("事件流已建立", "event", "sse.opened")
	// 在三十秒撤权目标内，为有界 IAM 调用预留五秒。
	interval := s.ReauthInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			next, err := s.IAM.Authorize(c.Request.Context(), c.Request, iamclient.PermissionConsoleEventsRead)
			if err != nil || !next.Allowed {
				reason = "access_revoked"
				if err != nil {
					reason = "reauthorization_failed"
				}
				logger.Warn("事件流重新鉴权未通过", "event", "sse.access_stopped", "reason", reason, "error", logging.Error(err))
				c.SSEvent("access_revoked", map[string]string{})
				c.Writer.Flush()
				return
			}
			if _, err = c.Writer.WriteString(": authorized\n\n"); err != nil {
				reason = "write_failed"
				logger.Debug("事件流发送失败", "event", "sse.write_failed", "error", logging.Error(err))
				return
			}
			c.Writer.Flush()
		}
	}
}

// principal 从 gin 上下文取出 Guard 写入的会话主体。
//
// 鉴权与审计已在中间件完成，handler 只读取所需信息。
func principal(c *gin.Context) (iamclient.CapabilityDecision, bool) {
	value, exists := c.Get(iamclient.PrincipalKey)
	if !exists {
		return iamclient.CapabilityDecision{}, false
	}

	decision, ok := value.(iamclient.CapabilityDecision)

	return decision, ok
}
