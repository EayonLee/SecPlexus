package repository

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/model"
)

// UserJSON 读取用户的公开字段投影。
func UserJSON(ctx context.Context, db *gorm.DB, id string, data *[]byte) error {
	var item model.APIUser
	if err := db.WithContext(ctx).Table("iam.users").Where("id = ?", id).Take(&item).Error; err != nil {
		return err
	}

	encoded, err := json.Marshal(item)
	*data = encoded
	return err
}

// VisibleOrganizations 读取当前用户可见的有效组织。
// VisibleOrganizations 返回当前身份可见的组织。
//
// 普通用户可见：自己所属组织，加上这些组织的祖先链（用于面包屑定位）；
// 不含兄弟节点与其后代。超管可见全树。父组织权限不会继承给用户。
func VisibleOrganizations(ctx context.Context, db *gorm.DB, userID uuid.UUID, isSuperAdmin bool) ([]model.APIOrganization, error) {
	items := []model.APIOrganization{}
	// 这里不返回 path：/me 只需要组织标识与层级，面包屑由前端用 parent_id 自行组装。
	err := db.WithContext(ctx).Raw(`
WITH RECURSIVE visible AS (
  SELECT o.id, o.slug, o.name, o.parent_id, o.depth, o.authz_epoch, o.created_at
  FROM iam.organizations o
  WHERE $2::boolean
     OR EXISTS(SELECT 1 FROM iam.organization_users m WHERE m.org_id=o.id AND m.user_id=$1 AND m.status='active')
  UNION
  SELECT p.id, p.slug, p.name, p.parent_id, p.depth, p.authz_epoch, p.created_at
  FROM iam.organizations p
  JOIN visible v ON p.id = v.parent_id
)
SELECT id::text, slug, name, parent_id::text, depth::int, authz_epoch::text
FROM visible ORDER BY depth, created_at, id LIMIT 500`, userID, isSuperAdmin).Scan(&items).Error
	return items, err
}
