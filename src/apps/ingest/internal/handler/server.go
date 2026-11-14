// Package handler 定义 Ingest 的 HTTP 协议边界。
package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// Server 持有 Ingest 处理请求所需的依赖。
// 骨架阶段没有业务路由；通道与连接器路由在后续批次挂载。
type Server struct {
	// Ready 报告外部依赖是否可用；nil 表示当前没有需要检查的依赖。
	// 通道与发布层落地后，bootstrap 在这里接入 Kafka/Redis 的真实探活，
	// 就绪检查绝不能在依赖全挂时仍然返回 200。
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

// 确认错误响应使用 HTTP 语义；result 包负责状态码映射。
var _ = http.StatusServiceUnavailable
