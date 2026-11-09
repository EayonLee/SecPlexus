package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/model"
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

	items, err := gorm.G[model.APIUser](db).Table("iam.users").Select("id,username,display_name,email,status,is_super_admin,auth_epoch").Where("? = '' OR username_normalized LIKE lower(?)||'%' OR display_name ILIKE '%'||?||'%'", query, query, query).Order("created_at,id").Limit(limit).Offset(offset).Find(ctx)
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

	// 显式取出 depth/parent_id：GORM 的 Select 投影在泛型 Find 下不会回填
	// 未列出的字段，曾导致列表里所有组织 depth 恒为 0。
	// 扫描类型与 OrganizationTree 保持一致：numeric 转 text、parent_id 允许为空字符串，
	// 避免驱动对 NULL/指针的处理差异导致字段静默为 0。
	type orgRow struct {
		AuthzEpoch string
		Depth      int
		ID         string
		Name       string
		ParentID   *string
		Slug       string
	}
	rows := []orgRow{}
	if err := db.WithContext(ctx).Raw(`
SELECT id::text, slug, name, parent_id::text, depth::int, authz_epoch::text
FROM iam.organizations ORDER BY created_at, id LIMIT ? OFFSET ?`, limit, offset).Scan(&rows).Error; err != nil {
		return nil, err
	}

	items := make([]model.APIOrganization, 0, len(rows))
	for _, r := range rows {
		epoch, err := strconv.ParseInt(r.AuthzEpoch, 10, 64)
		if err != nil {
			return nil, err
		}

		items = append(items, model.APIOrganization{
			AuthzEpoch: epoch,
			Depth:      r.Depth,
			ID:         r.ID,
			Name:       r.Name,
			ParentID:   r.ParentID,
			Slug:       r.Slug,
		})
	}

	return map[string]any{"items": items, "total": total, "limit": limit, "offset": offset}, nil
}

// ListOrganizationUsers 查询该业务的公开字段投影。
func ListOrganizationUsers(ctx context.Context, db *gorm.DB, limit, offset int, orgID string, subtree bool) (map[string]any, error) {
	scope := `m.org_id = $1`
	args := []any{orgID}
	if subtree {
		scope = `m.org_id IN (
			WITH RECURSIVE sub AS (
				SELECT o.id FROM iam.organizations o WHERE o.id = $1
				UNION
				SELECT c.id FROM iam.organizations c JOIN sub s ON c.parent_id = s.id
			) SELECT id FROM sub)`
	}

	sql := `SELECT jsonb_build_object(
			'id',m.id,'user_id',u.id,'is_super_admin',u.is_super_admin,
			'username',u.username,'display_name',u.display_name,'status',m.status,
			'org_id',m.org_id,'org_slug',o.slug,
			'source',CASE WHEN m.org_id = $1 THEN 'direct' ELSE 'subtree' END,
			'role_ids',COALESCE((SELECT jsonb_agg(mr.role_id ORDER BY mr.role_id)
				FROM iam.organization_user_roles mr
				WHERE mr.org_id=m.org_id AND mr.organization_user_id=m.id),'[]'::jsonb)
		),count(*) OVER()
		FROM iam.organization_users m
		JOIN iam.users u ON u.id=m.user_id
		JOIN iam.organizations o ON o.id=m.org_id
		WHERE ` + scope + `
		ORDER BY o.depth,m.created_at,m.id`

	return listJSON(ctx, db, limit, offset, sql, args...)
}

// ListRoleUsers 查询该业务的公开字段投影。
func ListRoleUsers(ctx context.Context, db *gorm.DB, limit, offset int, orgID, roleID string) (map[string]any, error) {
	return listJSON(
		ctx,
		db,
		limit,
		offset,
		`SELECT jsonb_build_object(
			'id',m.id,'user_id',u.id,'is_super_admin',u.is_super_admin,
			'username',u.username,'display_name',u.display_name,'status',m.status,
			'org_id',m.org_id,'org_slug',o.slug,
			'source',CASE WHEN m.org_id = $1 THEN 'direct' ELSE 'inherited' END
		),count(*) OVER()
		FROM iam.organization_users m
		JOIN iam.users u ON u.id=m.user_id
		JOIN iam.organizations o ON o.id=m.org_id
		JOIN iam.organization_user_roles mr ON mr.org_id=m.org_id AND mr.organization_user_id=m.id
		WHERE m.org_id=$1 AND mr.role_id=$2
		ORDER BY m.created_at,m.id`,
		orgID,
		roleID,
	)
}

const auditSQL = `SELECT jsonb_build_object('id',id,'occurred_at',occurred_at,'org_id',org_id,'actor_type',actor_type,'actor_name',actor_name,'action',action,'target_type',target_type,'target_id',target_id,'outcome',outcome,'reason_code',reason_code,'request_id',request_id,'metadata',metadata),count(*) OVER() FROM iam.audit_events`

// AuditFilter 是审计列表的服务端筛选条件。
// OrgIDs 为 nil 表示不限制组织（平台审计权限）；非 nil 时只返回这些组织的事件。
type AuditFilter struct {
	OrgIDs  []string
	Actor   string
	Outcome string
	From    string
	To      string
}

// ListAuditEvents 按筛选条件读取审计。可见范围由调用方的授权结果决定，
// 不接受客户端直接指定组织，避免越权扩大读取范围。
func ListAuditEvents(ctx context.Context, db *gorm.DB, filter AuditFilter, limit, offset int) (map[string]any, error) {
	where := []string{"TRUE"}
	args := []any{}

	if filter.OrgIDs != nil {
		where = append(where, fmt.Sprintf("org_id = ANY($%d)", len(args)+1))
		args = append(args, filter.OrgIDs)
	}

	if filter.Actor != "" {
		where = append(where, fmt.Sprintf("actor_name ILIKE $%d", len(args)+1))
		args = append(args, "%"+filter.Actor+"%")
	}

	if filter.Outcome != "" {
		where = append(where, fmt.Sprintf("outcome = $%d", len(args)+1))
		args = append(args, filter.Outcome)
	}

	if filter.From != "" {
		where = append(where, fmt.Sprintf("occurred_at >= $%d::date", len(args)+1))
		args = append(args, filter.From)
	}

	if filter.To != "" {
		where = append(where, fmt.Sprintf("occurred_at < ($%d::date + 1)", len(args)+1))
		args = append(args, filter.To)
	}

	sql := auditSQL + " WHERE " + strings.Join(where, " AND ") + " ORDER BY occurred_at DESC,id DESC"
	return listJSON(ctx, db, limit, offset, sql, args...)
}
