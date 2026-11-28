// Package handler 定义 Ingest 的 HTTP 协议边界。
package handler

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// Server 持有 Ingest 处理请求所需的依赖。
// 通道依赖走 router.Deps，管理 API 依赖在第五组加在这里——
// 同一依赖只保留一条装配路径，避免改一处忘另一处。
type Server struct {
	AuditOutboxHealth    func(context.Context) (int64, int64, error)
	SourceSnapshotAge    func() time.Duration
	KafkaOffsetsRejected func() uint64
	// Ready 报告外部依赖是否可用；nil 表示当前没有需要检查的依赖。
	Ready func(ctx context.Context) error
	// Stats 返回流水线计数；nil 表示指标端点不启用。
	Stats func() pipeline.Stats
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
