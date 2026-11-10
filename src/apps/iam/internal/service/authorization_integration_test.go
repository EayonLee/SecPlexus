//go:build integration

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/account"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authentication"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/config"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/testutil"
)

func domainFixture(t *testing.T) (*Service, context.Context) {
	t.Helper()
	store := testutil.Database(t)
	ctx := context.Background()
	secret := "isolated domain password"
	if err := store.Seed(ctx, "admin", secret); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}

	service := &Service{Store: store, Sessions: authentication.NewSessions(store, cfg), Config: cfg}
	return service, actorContext(t, service, "admin")
}

func actorContext(t *testing.T, service *Service, username string) context.Context {
	t.Helper()
	ctx := context.Background()
	user, err := repository.NewQueries(service.Store.DB).GetLoginUser(ctx, username)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := service.Sessions.Manager.Load(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	if _, err = service.Sessions.Create(loaded, user, uuid.NewString(), false); err != nil {
		t.Fatal(err)
	}

	if _, _, err = service.Sessions.Manager.Commit(loaded); err != nil {
		t.Fatal(err)
	}

	return loaded
}

func TestRoleUnionAndImmediateDelegationRevocation(t *testing.T) {
	s, admin := domainFixture(t)
	ctx := context.Background()
	member, err := s.CreateUser(admin, UserCreate{Username: "delegated", DisplayName: "委派管理员", Password: "temporary delegated password"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	actor := actorContext(t, s, "delegated")
	if _, err = s.Authorize(actor, "iam.audit.read"); !errors.Is(err, authorization.ErrDenied) {
		t.Fatal("new account unexpectedly has business grants")
	}

	managerRole, err := s.CreateRole(admin, RoleCreate{Name: "角色管理", Permissions: []string{"iam.roles.read", "iam.roles.manage", "iam.audit.read"}}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	readRole, err := s.CreateRole(admin, RoleCreate{Name: "用户读取", Permissions: []string{"iam.users.read"}}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	if err = s.ReplaceUserRoles(admin, member, UserRoles{RoleIDs: []string{managerRole, readRole}}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	for _, action := range []string{"iam.audit.read", "iam.users.read"} {
		if _, err = s.Authorize(actor, action); err != nil {
			t.Fatal(err)
		}
	}

	target, err := s.CreateRole(admin, RoleCreate{Name: "目标角色"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	// 先加载操作者快照，再让管理请求等待同一把数据库锁；
	// 等待期间提交撤权，验证请求拿到锁后会重新校验最新权限。
	if _, err = s.Authorize(actor, "iam.roles.manage"); err != nil {
		t.Fatal(err)
	}

	tx, err := s.Store.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}

	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, repository.ManagementLock); err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		close(started)
		finished <- s.ReplaceGrants(actor, target, RoleGrants{Permissions: []string{"iam.audit.read"}}, uuid.NewString())
	}()
	<-started
	if _, err = tx.Exec(ctx, `DELETE FROM iam.role_permissions WHERE role_id=$1 AND permission_key='iam.audit.read'`, managerRole); err != nil {
		t.Fatal(err)
	}

	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if err = <-finished; !errors.Is(err, authorization.ErrDenied) {
		t.Fatalf("queued delegation did not observe revocation: %v", err)
	}

	var count int
	if err = s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM iam.role_permissions WHERE role_id=$1`, target).Scan(&count); err != nil {
		t.Fatal(err)
	}

	if count != 0 {
		t.Fatal("revoked authority granted a permission")
	}

	if err = s.ReplaceUserRoles(admin, member, UserRoles{RoleIDs: []string{}}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	for _, action := range []string{"iam.roles.manage", "iam.users.read"} {
		if _, err = s.Authorize(actor, action); !errors.Is(err, authorization.ErrDenied) {
			t.Fatal("empty bindings retained a fallback grant")
		}
	}
}

// TestDeleteRoleRejectsWhenUsersBound 固定「角色仍有绑定用户时不可删除」。
// 数据库外键也会兜底，因此直接断言 service 返回 ErrRoleInUse。
func TestDeleteRoleRejectsWhenUsersBound(t *testing.T) {
	s, admin := domainFixture(t)
	boundUser, err := s.CreateUser(admin, UserCreate{Username: "bounduser", DisplayName: "绑定用户", Password: "temporary bound password"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	role, err := s.CreateRole(admin, RoleCreate{Name: "绑定角色", Permissions: []string{"iam.audit.read"}}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	if err = s.ReplaceUserRoles(admin, boundUser, UserRoles{RoleIDs: []string{role}}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	if err = s.DeleteRole(admin, role, uuid.NewString()); !errors.Is(err, repository.ErrRoleInUse) {
		t.Fatalf("delete role with bound users = %v, want ErrRoleInUse", err)
	}

	if err = s.ReplaceUserRoles(admin, boundUser, UserRoles{RoleIDs: []string{}}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	if err = s.DeleteRole(admin, role, uuid.NewString()); err != nil {
		t.Fatalf("delete unbound role = %v, want nil", err)
	}
}

func TestConfiguredAdministratorRejectsApplicationAndOperatorWrites(t *testing.T) {
	for _, operation := range []string{"disable", "password", "reset", "delete", "operator"} {
		t.Run(operation, func(t *testing.T) {
			s, actor := domainFixture(t)
			user, err := repository.NewQueries(s.Store.DB).GetLoginUser(context.Background(), "admin")
			if err != nil {
				t.Fatal(err)
			}

			switch operation {
			case "disable":
				err = s.UpdateUser(actor, user.ID.String(), UserUpdate{DisplayName: user.DisplayName, Status: enum.UserStatusDisabled}, uuid.NewString())
			case "password":
				err = s.ChangePassword(actor, "isolated domain password", "not from configuration", uuid.NewString())
			case "reset":
				err = s.ResetPassword(actor, user.ID.String(), "not from configuration", uuid.NewString())
			case "delete":
				err = s.DeleteUser(actor, user.ID.String(), uuid.NewString())
			case "operator":
				err = s.Store.RecoverPassword(context.Background(), user.ID, "not from configuration")
			}

			if !errors.Is(err, repository.ErrConfiguredUserImmutable) {
				t.Fatalf("configured administrator write %s was not rejected: %v", operation, err)
			}
		})
	}
}

// 内置角色的名称、说明与权限都来自启动同步，页面只能查看。
func TestBuiltinRoleIsImmutable(t *testing.T) {
	s, admin := domainFixture(t)
	var builtin string
	if err := s.Store.Pool.QueryRow(context.Background(), `SELECT id::text FROM iam.roles WHERE builtin`).Scan(&builtin); err != nil {
		t.Fatal(err)
	}

	if err := s.UpdateRole(admin, builtin, RoleUpdate{Name: "改名"}, uuid.NewString()); !errors.Is(err, repository.ErrBuiltinRoleImmutable) {
		t.Fatalf("builtin rename = %v", err)
	}

	if err := s.ReplaceGrants(admin, builtin, RoleGrants{Permissions: []string{"iam.audit.read"}}, uuid.NewString()); !errors.Is(err, repository.ErrBuiltinRoleImmutable) {
		t.Fatalf("builtin permission change = %v", err)
	}

	if err := s.DeleteRole(admin, builtin, uuid.NewString()); !errors.Is(err, repository.ErrBuiltinRoleImmutable) {
		t.Fatalf("builtin delete = %v", err)
	}

	var permissions int
	if err := s.Store.Pool.QueryRow(context.Background(), `SELECT count(*) FROM iam.role_permissions WHERE role_id=$1`, builtin).Scan(&permissions); err != nil {
		t.Fatal(err)
	}

	if permissions != 7 {
		t.Fatalf("builtin role permissions = %d, want 7", permissions)
	}
}

// 角色名全局唯一：创建与重命名都必须返回专门的冲突错误。
func TestRoleNameIsGloballyUnique(t *testing.T) {
	s, admin := domainFixture(t)
	if _, err := s.CreateRole(admin, RoleCreate{Name: "审计员"}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	if _, err := s.CreateRole(admin, RoleCreate{Name: " 审计员 "}, uuid.NewString()); !errors.Is(err, repository.ErrRoleNameTaken) {
		t.Fatalf("duplicate role name = %v", err)
	}

	if _, err := s.CreateRole(admin, RoleCreate{Name: "超级管理员"}, uuid.NewString()); !errors.Is(err, repository.ErrRoleNameTaken) {
		t.Fatalf("builtin role name reuse = %v", err)
	}

	other, err := s.CreateRole(admin, RoleCreate{Name: "响应员"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	if err = s.UpdateRole(admin, other, RoleUpdate{Name: "审计员"}, uuid.NewString()); !errors.Is(err, repository.ErrRoleNameTaken) {
		t.Fatalf("rename to existing role name = %v", err)
	}
}

// TestSeedReappliesBuiltinData 固定启动同步语义：权限与内置角色只增不删，
// 配置改名会收回旧账号的内置标记并让新账号接管。
func TestSeedReappliesBuiltinData(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()
	if err := store.Seed(ctx, "admin", "isolated seed password"); err != nil {
		t.Fatal(err)
	}

	var before uuid.UUID
	if err := store.Pool.QueryRow(ctx, `SELECT id FROM iam.users WHERE configured`).Scan(&before); err != nil {
		t.Fatal(err)
	}

	// 手工移除权限再重启同步，应补齐缺失的键且不删除其他授权。
	if _, err := store.Pool.Exec(ctx, `DELETE FROM iam.role_permissions WHERE permission_key='iam.audit.read' AND role_id IN (SELECT id FROM iam.roles WHERE builtin)`); err != nil {
		t.Fatal(err)
	}

	if err := store.Seed(ctx, "admin", "isolated seed password"); err != nil {
		t.Fatal(err)
	}

	var restored int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM iam.role_permissions WHERE permission_key='iam.audit.read' AND role_id IN (SELECT id FROM iam.roles WHERE builtin)`).Scan(&restored); err != nil {
		t.Fatal(err)
	}

	if restored != 1 {
		t.Fatal("seed did not restore a missing builtin permission")
	}

	// 改名：旧账号失去内置标记，新账号接管；旧账号不再持有内置角色。
	renamedUser := uuid.New()
	if err := repository.InsertUser(ctx, store.DB, renamedUser, "renamed", "renamed", "Renamed", "", false); err != nil {
		t.Fatal(err)
	}

	if err := store.Seed(ctx, "renamed", "isolated renamed password"); err != nil {
		t.Fatal(err)
	}

	var configured uuid.UUID
	if err := store.Pool.QueryRow(ctx, `SELECT id FROM iam.users WHERE configured`).Scan(&configured); err != nil {
		t.Fatal(err)
	}

	if configured != renamedUser {
		t.Fatalf("configured administrator = %s, want %s", configured, renamedUser)
	}

	var oldConfigured bool
	if err := store.Pool.QueryRow(ctx, `SELECT configured FROM iam.users WHERE id=$1`, before).Scan(&oldConfigured); err != nil {
		t.Fatal(err)
	}

	if oldConfigured {
		t.Fatal("previous configured account kept its builtin mark")
	}

	var oldBuiltin int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM iam.user_roles ur JOIN iam.roles r ON r.id=ur.role_id WHERE ur.user_id=$1 AND r.builtin`, before).Scan(&oldBuiltin); err != nil {
		t.Fatal(err)
	}

	if oldBuiltin != 0 {
		t.Fatal("previous configured account kept the builtin role")
	}

	// 新账号的密码是配置权威值。
	login, err := repository.NewQueries(store.DB).GetLoginUser(ctx, "renamed")
	if err != nil {
		t.Fatal(err)
	}

	if err = (account.PasswordHasher{}).CompareHashAndPassword(login.PasswordHash, "isolated renamed password"); err != nil {
		t.Fatal("configuration password was not applied to the renamed account")
	}
}

// 平台底线：任何变更都必须留下至少一名启用中的超级管理员。
// 操作者与目标必须分开：停用配置账号会同时结束它自己的会话。
func TestLastSuperAdminInvariant(t *testing.T) {
	s, admin := domainFixture(t)
	ctx := context.Background()
	builtin, err := repository.BuiltinRoleID(ctx, s.Store.DB)
	if err != nil {
		t.Fatal(err)
	}

	// 委派规则要求操作者持有被移除权限的全部键，因此这里授予完整目录。
	operatorRole, err := s.CreateRole(admin, RoleCreate{Name: "用户与角色管理员", Permissions: []string{
		"iam.users.read", "iam.users.manage", "iam.users.reset", "iam.users.roles",
		"iam.roles.read", "iam.roles.manage", "iam.audit.read",
	}}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	operator, err := s.CreateUser(admin, UserCreate{Username: "opsadmin", DisplayName: "运维管理员", Password: "temporary ops admin password"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	ops := actorContext(t, s, "opsadmin")
	if err = s.ReplaceUserRoles(admin, operator, UserRoles{RoleIDs: []string{operatorRole}}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	solo, err := s.CreateUser(admin, UserCreate{Username: "soloadmin", DisplayName: "唯一管理员", Password: "temporary solo admin password"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	if err = s.ReplaceUserRoles(admin, solo, UserRoles{RoleIDs: []string{builtin.String()}}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	// 配置管理员仍启用时，摘除 solo 的内置角色是安全的。
	if err = s.ReplaceUserRoles(ops, solo, UserRoles{RoleIDs: []string{}}, uuid.NewString()); err != nil {
		t.Fatalf("removing a super administrator role while another remains = %v, want nil", err)
	}

	if err = s.ReplaceUserRoles(admin, solo, UserRoles{RoleIDs: []string{builtin.String()}}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	// 构造“只剩一名启用中的超级管理员”的局面：直接停用配置账号（页面不允许，运维可以）。
	if _, err = s.Store.Pool.Exec(ctx, `UPDATE iam.users SET status='disabled' WHERE configured`); err != nil {
		t.Fatal(err)
	}

	if err = s.ReplaceUserRoles(ops, solo, UserRoles{RoleIDs: []string{}}, uuid.NewString()); !errors.Is(err, repository.ErrLastSuperAdmin) {
		t.Fatalf("removing the last active super administrator = %v, want ErrLastSuperAdmin", err)
	}

	if err = s.DeleteUser(ops, solo, uuid.NewString()); !errors.Is(err, repository.ErrLastSuperAdmin) {
		t.Fatalf("deleting the last active super administrator = %v, want ErrLastSuperAdmin", err)
	}

	if err = s.UpdateUser(ops, solo, UserUpdate{DisplayName: "唯一管理员", Status: enum.UserStatusDisabled}, uuid.NewString()); !errors.Is(err, repository.ErrLastSuperAdmin) {
		t.Fatalf("disabling the last active super administrator = %v, want ErrLastSuperAdmin", err)
	}
}

// 配置管理员的内置角色由部署配置保证，页面摘除会被拒绝。
func TestConfiguredAdministratorRolesAreLocked(t *testing.T) {
	s, admin := domainFixture(t)
	ctx := context.Background()
	builtin, err := repository.BuiltinRoleID(ctx, s.Store.DB)
	if err != nil {
		t.Fatal(err)
	}

	configured, err := repository.NewQueries(s.Store.DB).GetLoginUser(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}

	second, err := s.CreateUser(admin, UserCreate{Username: "keeper", DisplayName: "保留管理员", Password: "temporary keeper password"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	if err = s.ReplaceUserRoles(admin, second, UserRoles{RoleIDs: []string{builtin.String()}}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	// 即便还有另一名超级管理员，配置账号的内置角色也不能被页面摘除。
	if err = s.ReplaceUserRoles(admin, configured.ID.String(), UserRoles{RoleIDs: []string{}}, uuid.NewString()); !errors.Is(err, repository.ErrConfiguredUserImmutable) {
		t.Fatalf("removing builtin role from the configured account = %v, want ErrConfiguredUserImmutable", err)
	}
}

// 历史创建人引用不再阻塞删除：user_roles.created_by 置空，绑定本身保留。
func TestDeleteUserKeepsHistoricalRoleBindings(t *testing.T) {
	s, admin := domainFixture(t)
	ctx := context.Background()
	bound, err := s.CreateUser(admin, UserCreate{Username: "historyuser", DisplayName: "历史用户", Password: "temporary history password"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	role, err := s.CreateRole(admin, RoleCreate{Name: "历史角色", Permissions: []string{"iam.audit.read"}}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	// bound 用户创建了另一名用户的角色绑定，因此成为 created_by。
	target, err := s.CreateUser(admin, UserCreate{Username: "historytarget", DisplayName: "历史目标", Password: "temporary target password"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	boundCtx := actorContext(t, s, "historyuser")
	if err = s.ReplaceUserRoles(boundCtx, target, UserRoles{RoleIDs: []string{role}}, uuid.NewString()); !errors.Is(err, authorization.ErrDenied) {
		t.Fatalf("account without iam.users.roles changed roles: %v", err)
	}

	if err = s.ReplaceUserRoles(admin, target, UserRoles{RoleIDs: []string{role}}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	if err = s.DeleteUser(admin, bound, uuid.NewString()); err != nil {
		t.Fatalf("delete account that created role bindings = %v, want nil", err)
	}

	var remaining int
	if err = s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM iam.user_roles WHERE user_id=$1`, target).Scan(&remaining); err != nil {
		t.Fatal(err)
	}

	if remaining != 1 {
		t.Fatalf("target roles after creator deletion = %d, want 1", remaining)
	}
}
