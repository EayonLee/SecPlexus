package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/contracts/common"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// NewEngine 创建共享 Gin 边界；认证和业务保护由模块路由分组挂载。
func NewEngine() *gin.Engine {
	router := gin.New()
	router.HandleMethodNotAllowed = true
	router.RedirectTrailingSlash = false
	router.RedirectFixedPath = false
	_ = router.SetTrustedProxies(nil)
	router.Use(func(c *gin.Context) {
		id := uuid.NewString()
		ctx := logging.With(c.Request.Context(), "request_id", id, "method", c.Request.Method)
		c.Request = c.Request.WithContext(context.WithValue(ctx, requestKey{}, id))
		c.Header("X-Request-ID", id)
		started := time.Now()
		c.Next()
		level := slog.LevelInfo
		if c.FullPath() == "/health/ready" && c.Writer.Status() < 400 {
			level = slog.LevelDebug
		}
		logging.From(c.Request.Context()).Log(c.Request.Context(), level, "HTTP 请求完成",
			"event", "http.completed", "route", c.FullPath(), "status", c.Writer.Status(),
			"bytes", c.Writer.Size(), "duration_ms", time.Since(started).Milliseconds())
	})
	// nil 禁用 Gin 内置请求转储，避免 Cookie、原始 panic 和请求头进入日志。
	router.Use(gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, failure any) {
		logging.From(c.Request.Context()).Error("HTTP 请求异常", "event", "http.panic",
			"panic_kind", fmt.Sprintf("%T", failure), "stack", logging.Stack())
		c.Abort()
		if !c.Writer.Written() {
			result.Fail(c.Writer, common.CodeInternalError)
		}
	}))
	router.NoRoute(func(c *gin.Context) {
		result.Fail(c.Writer, common.CodeNotFound)
	})
	router.NoMethod(func(c *gin.Context) {
		result.Fail(c.Writer, common.CodeMethodNotAllowed)
	})
	return router
}
