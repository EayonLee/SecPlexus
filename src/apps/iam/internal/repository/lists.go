package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository/sqlcgen"
)

// UserSummary 是账号的公开投影：凭据不在其中，角色是直接持有的绑定。
type UserSummary struct {
	ID          uuid.UUID       // 资源标识。
	Username    string          // 登录账号。
	DisplayName string          // 界面显示名称。
	Email       *string         // 邮箱；空表示未设置。
	Status      enum.UserStatus // 当前启用状态，取值见 enum.UserStatus。
	Configured  bool            // 是否为配置托管的内置管理员。
	AuthEpoch   int64           // 账号版本。
	Roles       []RoleRef       // 直接持有的角色，按名称排序。
}

// UserPage 是一页用户；Total 是同一筛选条件下的总行数。
type UserPage struct {
	Items  []UserSummary // 当前页，顺序与查询一致。
	Total  int64         // 满足筛选条件的总行数。
	Limit  int           // 请求的页大小。
	Offset int           // 请求的偏移量。
}

// RolePage 是一页角色；Total 是角色总数。
type RolePage struct {
	Items  []Role // 当前页，内置角色排在前面。
	Total  int64  // 角色总数。
	Limit  int    // 请求的页大小。
	Offset int    // 请求的偏移量。
}

// AuditEvent 是一条审计事实。游标边界就是 OccurredAt 与 ID，服务端自己编码。
type AuditEvent struct {
	ID         uuid.UUID       // 事件标识。
	OccurredAt time.Time       // 事件时间，数据库时钟。
	ActorType  string          // 触发者类型，取值见 enum.ActorType。
	ActorName  *string         // 触发者名称；系统事件为空。
	Action     string          // 稳定动作名，形如 iam.auth.login。
	TargetType string          // 目标类型。
	TargetID   *uuid.UUID      // 目标标识；无具体目标时为空。
	Outcome    string          // 结果，取值见 enum.AuditOutcome。
	ReasonCode *string         // 拒绝或失败原因；成功时为空。
	RequestID  string          // 关联请求标识。
	Metadata   json.RawMessage // 结构化扩展值，大小受限。
}

// likePattern 把用户输入转成字面量模糊匹配：% 与 _ 不再被当作通配符。
func likePattern(value string) string {
	escaped := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(value)
	return "%" + escaped + "%"
}

// ListUsers 按用户名、显示名或邮箱模糊搜索用户，并附带角色投影。
func ListUsers(ctx context.Context, db *gorm.DB, limit, offset int, query string) (UserPage, error) {
	pattern := ""
	if query != "" {
		pattern = likePattern(query)
	}

	rows, err := queries(db).ListUsers(ctx, sqlcgen.ListUsersParams{
		Query:      pattern,
		PageLimit:  int32(limit),
		PageOffset: int32(offset),
	})
	if err != nil {
		return UserPage{}, err
	}

	page := UserPage{Items: make([]UserSummary, 0, len(rows)), Limit: limit, Offset: offset}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		page.Total = row.Total
		item := UserSummary{
			ID:          row.ID,
			Username:    row.Username,
			DisplayName: row.DisplayName,
			Status:      row.Status,
			Configured:  row.Configured,
			AuthEpoch:   row.AuthEpoch,
		}

		if row.Email.Valid {
			item.Email = &row.Email.String
		}

		page.Items = append(page.Items, item)
		ids = append(ids, row.ID)
	}

	roles, err := rolesByUserIDs(ctx, db, ids)
	if err != nil {
		return UserPage{}, err
	}

	for i := range page.Items {
		page.Items[i].Roles = roles[page.Items[i].ID]
	}

	return page, nil
}

// UserByID 读取单个用户的公开投影；不存在时返回 sql.ErrNoRows。
func UserByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (UserSummary, error) {
	row, err := queries(db).GetUserSummary(ctx, id)
	if err != nil {
		return UserSummary{}, err
	}

	user := UserSummary{
		ID:          row.ID,
		Username:    row.Username,
		DisplayName: row.DisplayName,
		Status:      row.Status,
		Configured:  row.Configured,
		AuthEpoch:   row.AuthEpoch,
	}

	if row.Email.Valid {
		user.Email = &row.Email.String
	}

	roles, err := rolesByUserIDs(ctx, db, []uuid.UUID{id})
	if err != nil {
		return UserSummary{}, err
	}

	user.Roles = roles[id]

	return user, nil
}

// rolesByUserIDs 批量读取页内用户的角色绑定，避免逐行查询。
func rolesByUserIDs(ctx context.Context, db *gorm.DB, ids []uuid.UUID) (map[uuid.UUID][]RoleRef, error) {
	grouped := make(map[uuid.UUID][]RoleRef, len(ids))
	if len(ids) == 0 {
		return grouped, nil
	}

	values, err := uuidArray(ids)
	if err != nil {
		return nil, err
	}

	rows, err := queries(db).UserRolesByUserIDs(ctx, values)
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		grouped[row.UserID] = append(grouped[row.UserID], RoleRef{ID: row.RoleID.String(), Name: row.RoleName})
	}

	return grouped, nil
}

// ListRoles 返回全局角色，内置角色也在列表中且排序稳定。
func ListRoles(ctx context.Context, db *gorm.DB, limit, offset int) (RolePage, error) {
	rows, err := queries(db).ListRoles(ctx, sqlcgen.ListRolesParams{PageLimit: int32(limit), PageOffset: int32(offset)})
	if err != nil {
		return RolePage{}, err
	}

	page := RolePage{Items: make([]Role, 0, len(rows)), Limit: limit, Offset: offset}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		page.Total = row.Total
		page.Items = append(page.Items, Role{
			ID:          row.ID.String(),
			Name:        row.Name,
			Description: row.Description,
			Builtin:     row.Builtin,
			UserCount:   row.UserCount,
		})
		ids = append(ids, row.ID)
	}

	granted, err := permissionsByRoleIDs(ctx, db, ids)
	if err != nil {
		return RolePage{}, err
	}

	for i := range page.Items {
		page.Items[i].Permissions = granted[ids[i]]
	}

	return page, nil
}

// RoleByID 读取单个角色的公开投影；不存在时返回 sql.ErrNoRows。
func RoleByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (Role, error) {
	row, err := queries(db).GetRoleSummary(ctx, id)
	if err != nil {
		return Role{}, err
	}

	role := Role{
		ID:          row.ID.String(),
		Name:        row.Name,
		Description: row.Description,
		Builtin:     row.Builtin,
		UserCount:   row.UserCount,
	}

	granted, err := permissionsByRoleIDs(ctx, db, []uuid.UUID{id})
	if err != nil {
		return Role{}, err
	}

	role.Permissions = granted[id]

	return role, nil
}

// permissionsByRoleIDs 批量读取页内角色的权限键。
func permissionsByRoleIDs(ctx context.Context, db *gorm.DB, ids []uuid.UUID) (map[uuid.UUID][]string, error) {
	grouped := make(map[uuid.UUID][]string, len(ids))
	if len(ids) == 0 {
		return grouped, nil
	}

	values, err := uuidArray(ids)
	if err != nil {
		return nil, err
	}

	rows, err := queries(db).RolePermissionsByRoleIDs(ctx, values)
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		grouped[row.RoleID] = append(grouped[row.RoleID], row.PermissionKey)
	}

	return grouped, nil
}

// AuditFilter 是审计列表的服务端筛选条件；可见范围由 iam.audit.read 决定，没有组织维度。
type AuditFilter struct {
	Actor   string // 触发者名称模糊匹配；空串表示不筛选。
	Outcome string // 结果精确匹配；空串表示不筛选。
	From    string // 起始日期（含），格式 YYYY-MM-DD；空串表示不限。
	To      string // 结束日期（含）；空串表示不限。
}

// AuditPage 是审计列表的一页；NextCursor 为空表示没有更早的事件。
type AuditPage struct {
	Items      []AuditEvent // 当前页，按时间倒序。
	NextCursor string       // 下一页游标；空表示已到最早一条。
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
	page := AuditPage{Items: []AuditEvent{}}
	params := sqlcgen.ListAuditEventsParams{
		Outcome:   filter.Outcome,
		FromDate:  filter.From,
		ToDate:    filter.To,
		PageLimit: int32(limit + 1),
	}

	if filter.Actor != "" {
		params.Actor = likePattern(filter.Actor)
	}

	if cursor != "" {
		at, id, err := decodeAuditCursor(cursor)
		if err != nil {
			return page, err
		}

		params.CursorAt = at
		params.CursorID = id
	}

	rows, err := queries(db).ListAuditEvents(ctx, params)
	if err != nil {
		return page, err
	}

	// 多取一行只用于判断是否还有下一页，返回给客户端的仍是 limit 条。
	boundaries := make([]string, 0, len(rows))
	for _, row := range rows {
		event := AuditEvent{
			ID:         row.ID,
			OccurredAt: row.OccurredAt,
			ActorType:  row.ActorType,
			Action:     row.Action,
			TargetType: row.TargetType,
			TargetID:   row.TargetID,
			Outcome:    row.Outcome,
			RequestID:  row.RequestID,
			Metadata:   row.Metadata,
		}

		if row.ActorName.Valid {
			event.ActorName = &row.ActorName.String
		}

		if row.ReasonCode.Valid {
			event.ReasonCode = &row.ReasonCode.String
		}

		page.Items = append(page.Items, event)
		boundaries = append(boundaries, encodeAuditCursor(row.OccurredAt, row.ID.String()))
	}

	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.NextCursor = boundaries[limit-1]
	}

	return page, nil
}
