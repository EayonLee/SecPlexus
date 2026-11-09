package management

import (
	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/model"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// ListAuditEvents 读取审计日志。可见范围由业务用例按授权结果裁剪，
// 客户端只能收窄结果（操作者、时间范围、结果），不能扩大范围。
func (s *Handler) ListAuditEvents(c *gin.Context) {
	var params model.AuditQuery
	if err := c.ShouldBindQuery(&params); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}

	response, err := s.Service.ListAuditEvents(c.Request.Context(), params)
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Success(c.Writer, response)
}
