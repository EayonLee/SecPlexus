package management

import (
	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/router"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"

	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// pagination 只补充协议默认值；格式与范围由生成绑定器和OpenAPI负责。
func pagination(limit, offset *int) (int, int) {
	size, start := 50, 0
	if limit != nil {
		size = *limit
	}
	if offset != nil {
		start = *offset
	}
	return size, start
}

// Permissions 返回当前组织可查看的权限目录。
func (s *Handler) Permissions(c *gin.Context, params router.PermissionsParams) {
	w := c.Writer
	r := c.Request
	orgID := ""
	if params.OrgId != nil {
		orgID = *params.OrgId
	}
	items, err := s.Service.PermissionCatalog(r.Context(), orgID)
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Success(w, map[string]any{"items": items})
}
