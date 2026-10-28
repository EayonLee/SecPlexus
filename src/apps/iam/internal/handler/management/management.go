package management

import (
	"net/http"
	"strconv"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// pagination 绑定并限制列表页大小与偏移量。
func pagination(r *http.Request) (int, int, error) {
	limit, offset := 50, 0
	for key, target := range map[string]*int{"limit": &limit, "offset": &offset} {
		if value := r.URL.Query().Get(key); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return 0, 0, repository.ErrInvalid
			}

			*target = parsed
		}
	}

	if limit < 1 || limit > 100 || offset < 0 || offset > 100000 {
		return 0, 0, repository.ErrInvalid
	}

	return limit, offset, nil
}

// permissions 返回当前组织可查看的权限目录。
func (s *Handler) permissions(w http.ResponseWriter, r *http.Request) error {
	items, err := s.Service.PermissionCatalog(r.Context(), r.URL.Query().Get("org_id"))
	if err != nil {
		return err
	}

	result.Success(w, map[string]any{"items": items})
	return nil
}
