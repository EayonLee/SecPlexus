package repository

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
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

// likePattern 把用户输入转成字面量模糊匹配：% 与 _ 不再被当作通配符。
func likePattern(value string) string {
	escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(value)
	return "%" + escaped + "%"
}

// userProjection 是用户公开投影：角色直接来自数据库。
const userProjection = `jsonb_build_object(
	'id',u.id::text,'username',u.username,'display_name',u.display_name,'email',u.email,
	'status',u.status,'configured',u.configured,'auth_epoch',u.auth_epoch::text,
	'roles',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',r.id::text,'name',r.name) ORDER BY r.name,r.id)
		FROM iam.user_roles ur JOIN iam.roles r ON r.id=ur.role_id WHERE ur.user_id=u.id),'[]'::jsonb)
)`

// userSelectSQL 在投影后附加窗口计数，供列表分页使用。
const userSelectSQL = "SELECT " + userProjection + ",count(*) OVER() FROM iam.users u"

// ListUsers 按用户名、显示名或邮箱模糊搜索用户，并附带角色投影。
func ListUsers(ctx context.Context, db *gorm.DB, limit, offset int, query string) (map[string]any, error) {
	return listJSON(ctx, db, limit, offset,
		userSelectSQL+`
		WHERE ($1 = '' OR u.username ILIKE $1 ESCAPE E'\\' OR u.display_name ILIKE $1 ESCAPE E'\\' OR COALESCE(u.email,'') ILIKE $1 ESCAPE E'\\')
		ORDER BY u.username,u.id`, likePattern(query))
}

// UserJSON 读取单个用户的公开投影；不存在时返回 sql.ErrNoRows。
func UserJSON(ctx context.Context, db *gorm.DB, id string) ([]byte, error) {
	var item []byte
	err := db.WithContext(ctx).Raw("SELECT "+userProjection+` FROM iam.users u WHERE u.id=$1`, id).Row().Scan(&item)
	if err == sql.ErrNoRows {
		return nil, gorm.ErrRecordNotFound
	}

	return item, err
}

// roleProjection 是角色公开投影：内置标记、权限键与持有用户数。
const roleProjection = `jsonb_build_object(
	'id',r.id::text,'name',r.name,'description',r.description,'builtin',r.builtin,
	'permissions',COALESCE((SELECT jsonb_agg(permission_key ORDER BY permission_key) FROM iam.role_permissions rp WHERE rp.role_id=r.id),'[]'::jsonb),
	'user_count',(SELECT count(*) FROM iam.user_roles ur WHERE ur.role_id=r.id)
)`

// roleSelectSQL 在投影后附加窗口计数，供列表分页使用。
const roleSelectSQL = "SELECT " + roleProjection + ",count(*) OVER() FROM iam.roles r"

// ListRoles 返回全局角色，内置角色也在列表中且排序稳定。
func ListRoles(ctx context.Context, db *gorm.DB, limit, offset int) (map[string]any, error) {
	return listJSON(ctx, db, limit, offset, roleSelectSQL+` ORDER BY r.builtin DESC,r.name,r.id`)
}

// RoleJSON 读取单个角色的公开投影；不存在时返回 gorm.ErrRecordNotFound。
func RoleJSON(ctx context.Context, db *gorm.DB, id string) ([]byte, error) {
	var item []byte
	if err := db.WithContext(ctx).Raw("SELECT "+roleProjection+` FROM iam.roles r WHERE r.id=$1`, id).Row().Scan(&item); err != nil {
		if err == sql.ErrNoRows {
			return nil, gorm.ErrRecordNotFound
		}

		return nil, err
	}

	return item, nil
}

// auditProjection 是审计公开投影。游标列单独返回，客户端看不到原始边界值。
const auditProjection = `jsonb_build_object('id',id,'occurred_at',occurred_at,'actor_type',actor_type,'actor_name',actor_name,'action',action,'target_type',target_type,'target_id',target_id,'outcome',outcome,'reason_code',reason_code,'request_id',request_id,'metadata',metadata)`

const auditSelectSQL = "SELECT " + auditProjection + ",occurred_at,id FROM iam.audit_events"

// AuditFilter 是审计列表的服务端筛选条件；可见范围由 iam.audit.read 决定，没有组织维度。
type AuditFilter struct {
	Actor   string
	Outcome string
	From    string
	To      string
}

// AuditPage 是审计列表的一页；NextCursor 为空表示没有更早的事件。
type AuditPage struct {
	Items      []json.RawMessage
	NextCursor string
}

// encodeAuditCursor 把排序键 (occurred_at,id) 编码成不透明游标。
func encodeAuditCursor(at time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(at.UTC().Format(time.RFC3339Nano) + "|" + id))
}

// decodeAuditCursor 只接受本服务签发的游标形状；非法值按无效输入拒绝。
func decodeAuditCursor(value string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return time.Time{}, "", ErrInvalid
	}

	at, id, found := strings.Cut(string(raw), "|")
	if !found {
		return time.Time{}, "", ErrInvalid
	}

	parsed, err := time.Parse(time.RFC3339Nano, at)
	// occurred_at 始终由数据库默认值写入，零值游标不是本服务签发过的边界。
	if err != nil || parsed.IsZero() {
		return time.Time{}, "", ErrInvalid
	}

	if _, err = uuid.Parse(id); err != nil {
		return time.Time{}, "", ErrInvalid
	}

	return parsed, id, nil
}

// ListAuditEvents 按筛选条件读取审计，用 (occurred_at,id) 键集分页。
// 审计行数与时间同向增长，OFFSET 深翻会随历史线性变慢，因此只提供向前游标。
func ListAuditEvents(ctx context.Context, db *gorm.DB, filter AuditFilter, limit int, cursor string) (AuditPage, error) {
	page := AuditPage{Items: []json.RawMessage{}}
	where := []string{"TRUE"}
	args := []any{}

	if filter.Actor != "" {
		where = append(where, fmt.Sprintf("actor_name ILIKE $%d ESCAPE E'\\\\'", len(args)+1))
		args = append(args, likePattern(filter.Actor))
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

	if cursor != "" {
		at, id, err := decodeAuditCursor(cursor)
		if err != nil {
			return page, err
		}

		where = append(where, fmt.Sprintf("(occurred_at,id) < ($%d,$%d)", len(args)+1, len(args)+2))
		args = append(args, at, id)
	}

	// 多取一行只用于判断是否还有下一页，返回给客户端的仍是 limit 条。
	statement := auditSelectSQL + " WHERE " + strings.Join(where, " AND ") +
		fmt.Sprintf(" ORDER BY occurred_at DESC,id DESC LIMIT $%d", len(args)+1)
	args = append(args, limit+1)
	rows, err := db.WithContext(ctx).Raw(statement, args...).Rows()
	if err != nil {
		return page, err
	}

	defer func() { _ = rows.Close() }()

	boundaries := []string{}
	for rows.Next() {
		var item []byte
		var occurredAt time.Time
		var id uuid.UUID
		if err = rows.Scan(&item, &occurredAt, &id); err != nil {
			return page, err
		}

		page.Items = append(page.Items, item)
		boundaries = append(boundaries, encodeAuditCursor(occurredAt, id.String()))
	}

	if err = rows.Err(); err != nil {
		return page, err
	}

	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.NextCursor = boundaries[limit-1]
	}

	return page, nil
}
