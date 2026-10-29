package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
)

// listJSON 复用模块内固定查询的分页与总数计算，所有外部值均使用绑定参数。
func listJSON(ctx context.Context, db *gorm.DB, limit, offset int, sql string, args ...any) (map[string]any, error) {
	unpagedSQL := sql
	unpagedArgs := append([]any(nil), args...)
	sql += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, limit, offset)
	rows, err := db.WithContext(ctx).Raw(sql, args...).Rows()
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	items := []json.RawMessage{}
	var total int64
	for rows.Next() {
		var item []byte
		if err = rows.Scan(&item, &total); err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	if len(items) == 0 && offset > 0 {
		if err = db.WithContext(ctx).Raw("SELECT count(*) FROM ("+unpagedSQL+") AS matches", unpagedArgs...).Row().Scan(&total); err != nil {
			return nil, err
		}
	}

	return map[string]any{"items": items, "total": total, "limit": limit, "offset": offset}, nil
}

// ListUsers 查询该业务的公开字段投影。
func ListUsers(ctx context.Context, db *gorm.DB, limit, offset int, query string) (map[string]any, error) {
	filter := db.WithContext(ctx).Table("iam.users").Where("? = '' OR username_normalized LIKE lower(?)||'%' OR display_name ILIKE '%'||?||'%'", query, query, query)
	var total int64
	if err := filter.Count(&total).Error; err != nil {
		return nil, err
	}

	items, err := gorm.G[contract.APIUser](db).Table("iam.users").Select("id,username,display_name,email,status,is_super_admin,auth_version").Where("? = '' OR username_normalized LIKE lower(?)||'%' OR display_name ILIKE '%'||?||'%'", query, query, query).Order("created_at,id").Limit(limit).Offset(offset).Find(ctx)
	if err != nil {
		return nil, err
	}

	return map[string]any{"items": items, "total": total, "limit": limit, "offset": offset}, nil
}

// ListOrganizations 查询该业务的公开字段投影。
func ListOrganizations(ctx context.Context, db *gorm.DB, limit, offset int) (map[string]any, error) {
	filter := db.WithContext(ctx).Table("iam.organizations")
	var total int64
	if err := filter.Count(&total).Error; err != nil {
		return nil, err
	}

	items, err := gorm.G[contract.APIOrganization](db).Table("iam.organizations").Select("id,slug,name,status,authz_version").Order("created_at,id").Limit(limit).Offset(offset).Find(ctx)
	if err != nil {
		return nil, err
	}

	return map[string]any{"items": items, "total": total, "limit": limit, "offset": offset}, nil
}

// ListMembers 查询该业务的公开字段投影。
func ListMembers(ctx context.Context, db *gorm.DB, limit, offset int, orgID string) (map[string]any, error) {
	return listJSON(
		ctx,
		db,
		limit,
		offset,
		`SELECT jsonb_build_object('id',m.id,'user_id',u.id,'username',u.username,'display_name',u.display_name,'status',m.status,'role_ids',COALESCE((SELECT jsonb_agg(mr.role_id ORDER BY mr.role_id) FROM iam.membership_roles mr WHERE mr.org_id=m.org_id AND mr.membership_id=m.id),'[]'::jsonb)),count(*) OVER() FROM iam.memberships m JOIN iam.users u ON u.id=m.user_id WHERE m.org_id=$1 ORDER BY m.created_at,m.id`,
		orgID,
	)
}

// ListRoleMembers 查询该业务的公开字段投影。
func ListRoleMembers(ctx context.Context, db *gorm.DB, limit, offset int, orgID, roleID string) (map[string]any, error) {
	return listJSON(
		ctx,
		db,
		limit,
		offset,
		`SELECT jsonb_build_object('id',m.id,'user_id',u.id,'username',u.username,'display_name',u.display_name,'status',m.status),count(*) OVER() FROM iam.memberships m JOIN iam.users u ON u.id=m.user_id JOIN iam.membership_roles mr ON mr.org_id=m.org_id AND mr.membership_id=m.id WHERE m.org_id=$1 AND mr.role_id=$2 ORDER BY m.created_at,m.id`,
		orgID,
		roleID,
	)
}

const auditSQL = `SELECT jsonb_build_object('id',id,'occurred_at',occurred_at,'org_id',org_id,'actor_type',actor_type,'actor_name',actor_name,'action',action,'target_type',target_type,'target_id',target_id,'outcome',outcome,'reason_code',reason_code,'request_id',request_id,'metadata',metadata),count(*) OVER() FROM iam.audit_events`

// ListAuditEvents 仅按调用方授权后的范围读取审计，不公开内部凭据。
func ListAuditEvents(ctx context.Context, db *gorm.DB, orgID string, limit, offset int) (map[string]any, error) {
	if orgID != "" {
		return listJSON(ctx, db, limit, offset, auditSQL+` WHERE org_id=$1 ORDER BY occurred_at DESC,id DESC`, orgID)
	}

	return listJSON(ctx, db, limit, offset, auditSQL+` WHERE org_id IS NULL ORDER BY occurred_at DESC,id DESC`)
}
