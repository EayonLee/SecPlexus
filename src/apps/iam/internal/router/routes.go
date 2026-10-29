// Package router 挂载生成的公开路由；内部路由独立挂载。
package router

import (
	"github.com/EayonLee/SecPlexus/src/contracts/common"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
	"github.com/gin-gonic/gin"
)

// RegisterPublic 让契约生成器负责业务路由，健康检查由模块单独提供。
func RegisterPublic(engine gin.IRouter, server ServerInterface, ready gin.HandlerFunc) {
	engine.GET("/health/ready", ready)
	RegisterHandlersWithOptions(engine, server, GinServerOptions{ErrorHandler: func(c *gin.Context, _ error, _ int) { result.Fail(c.Writer, common.CodeInvalidRequest); c.Abort() }})
}
