package repository

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
)

// OrganizationJSON 读取组织的公开字段投影。
func OrganizationJSON(ctx context.Context, db *gorm.DB, org string, data *[]byte) error {
	var item contract.APIOrganization
	if err := db.WithContext(ctx).Table("iam.organizations").Where("id = ?", org).Take(&item).Error; err != nil {
		return err
	}

	encoded, err := json.Marshal(item)
	*data = encoded
	return err
}

// UserJSON 读取用户的公开字段投影。
func UserJSON(ctx context.Context, db *gorm.DB, id string, data *[]byte) error {
	var item contract.APIUser
	if err := db.WithContext(ctx).Table("iam.users").Where("id = ?", id).Take(&item).Error; err != nil {
		return err
	}

	encoded, err := json.Marshal(item)
	*data = encoded
	return err
}

// VisibleOrganizations 读取当前用户可见的有效组织。
func VisibleOrganizations(ctx context.Context, db *gorm.DB, userID uuid.UUID, isSuperAdmin bool) (*sql.Rows, error) {
	return db.WithContext(ctx).Raw(`SELECT o.id::text,o.slug,o.name,o.authz_version::text FROM iam.organizations o WHERE o.status='active' AND ($2::boolean OR EXISTS(SELECT 1 FROM iam.memberships m WHERE m.org_id=o.id AND m.user_id=$1 AND m.status='active')) ORDER BY o.created_at,o.id LIMIT 500`, userID, isSuperAdmin).Rows()
}
