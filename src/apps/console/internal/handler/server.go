// Package handler 按 IAM 授权结果处理 Console 请求，不信任浏览器声明的身份。
package handler

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/console/internal/model"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// Server 使用 IAM 的授权结果处理 Console 请求。
type Server struct {
	IAM *iamclient.Client // 用于查询 IAM 权限目录。
}

// Live 报告 Console HTTP 进程可响应。
func (s *Server) Live(c *gin.Context) {
	result.Success(c.Writer, map[string]bool{"live": true})
}

// Ready 核对 Console 的服务凭据能访问 IAM，且依赖的权限已登记。
func (s *Server) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := s.IAM.ValidateActions(ctx, iamclient.PermissionConsoleContextRead); err != nil {
		result.Fail(c.Writer, result.CodeServiceUnavailable)
		return
	}

	result.Success(c.Writer, map[string]bool{"ready": true})
}

// Context 返回已经 IAM 验证的当前身份上下文。
func (s *Server) Context(c *gin.Context) {
	w := c.Writer
	decision, ok := principal(c)
	if !ok {
		result.Fail(w, result.CodeUnauthenticated)

		return
	}

	result.Success(w, model.Context{
		UserID:     decision.Principal.UserID,
		DecisionID: decision.DecisionID,
	})
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
