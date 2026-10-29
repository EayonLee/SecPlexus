// Package router 挂载 Console 生成路由和原生流式路由。
package router

import (
	"github.com/EayonLee/SecPlexus/src/contracts/common"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
	"github.com/gin-gonic/gin"
)

// Register 保留 SSE 的原生响应和取消语义，不套普通 JSON 包装器。
func Register(engine gin.IRouter, server ServerInterface, events gin.HandlerFunc) {
	engine.GET("/health/ready", func(c *gin.Context) { result.Success(c.Writer, map[string]bool{"ready": true}) })
	RegisterHandlersWithOptions(engine, server, GinServerOptions{ErrorHandler: func(c *gin.Context, _ error, _ int) { result.Fail(c.Writer, common.CodeInvalidRequest); c.Abort() }})
	engine.GET("/api/v1/console/orgs/:org_id/events", events)
}
