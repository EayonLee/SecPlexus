package management

import (
	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// Permissions 返回扁平权限目录及当前操作者的可委派范围。
func (s *Handler) Permissions(c *gin.Context) {
	w := c.Writer
	r := c.Request
	items, err := s.Service.PermissionCatalog(r.Context())
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Success(w, map[string]any{"items": items})
}
