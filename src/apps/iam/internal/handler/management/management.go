package management

import (
	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/model"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// Permissions 返回当前组织可查看的权限目录。
func (s *Handler) Permissions(c *gin.Context) {
	var params model.OrganizationQuery
	if err := c.ShouldBindQuery(&params); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}
	w := c.Writer
	r := c.Request
	orgID := params.OrgID
	items, err := s.Service.PermissionCatalog(r.Context(), orgID)
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Success(w, map[string]any{"items": items})
}
