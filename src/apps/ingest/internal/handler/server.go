// Package handler 定义 Ingest 的 HTTP 协议边界。
package handler

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connector"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// Server 持有 Ingest 处理请求所需的依赖。
type Server struct {
	Registry connector.Registry // 接入点查询；由通道与管理 API 共用。
	Pipeline *pipeline.Pipeline // 已受理事件的解析与发布。
	// Ready 报告外部依赖是否可用；nil 表示当前没有需要检查的依赖。
	Ready func(ctx context.Context) error
}

// ReadyHandler 就绪检查：依赖不可用时返回 503，负载均衡据此摘除实例。
func (s *Server) ReadyHandler(c *gin.Context) {
	if s.Ready != nil {
		if err := s.Ready(c.Request.Context()); err != nil {
			result.Fail(c.Writer, result.CodeServiceUnavailable)

			return
		}
	}

	result.Success(c.Writer, map[string]bool{"ready": true})
}
