//go:build integration

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authentication"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/config"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/testutil"
)

func domainFixture(t *testing.T) (*Service, context.Context, string) {
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
	var org string
	if err = store.Pool.QueryRow(ctx, `SELECT id::text FROM iam.organizations WHERE parent_id IS NULL`).Scan(&org); err != nil {
		t.Fatal(err)
	}

	return service, actorContext(t, service, "admin"), org
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

	if _, err = service.Sessions.Create(loaded, user, uuid.NewString(), "integration", "127.0.0.1", false); err != nil {
		t.Fatal(err)
	}

	if _, _, err = service.Sessions.Manager.Commit(loaded); err != nil {
		t.Fatal(err)
	}

	return loaded
}

func authzVersion(t *testing.T, s *Service, org string) int64 {
	t.Helper()
	var v int64
	if err := s.Store.Pool.QueryRow(context.Background(), `SELECT authz_epoch FROM iam.organizations WHERE id=$1`, org).Scan(&v); err != nil {
		t.Fatal(err)
	}

	return v
}

func TestCustomRoleUnionEmptyGrantsAndImmediateDelegationRevocation(t *testing.T) {
	s, admin, org := domainFixture(t)
	ctx := context.Background()
	member, err := s.CreateOrganizationUser(admin, org, OrganizationUserInput{Username: "delegated", DisplayName: "委派管理员", Password: "temporary delegated password"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	actor := actorContext(t, s, "delegated")
	if _, err = s.Authorize(actor, "iam.audit.read", org); !errors.Is(err, authorization.ErrDenied) {
		t.Fatal("default member unexpectedly has business grants")
	}

	grants := []string{"iam.roles.read", "iam.roles.manage", "iam.roles.assign", "iam.audit.read"}
	managerRole, err := s.CreateRole(admin, org, RoleCreateInput{Name: "角色管理", Permissions: grants}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	readRole, err := s.CreateRole(admin, org, RoleCreateInput{Name: "用户读取", Permissions: []string{"iam.users.read"}}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	if err = s.ReplaceOrganizationUserRoles(admin, org, member, OrganizationUserRolesInput{RoleIDs: []string{managerRole, readRole}, AuthzEpoch: authzVersion(t, s, org)}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	for _, action := range []string{"iam.audit.read", "iam.users.read"} {
		if _, err = s.Authorize(actor, action, org); err != nil {
			t.Fatal(err)
		}
	}

	target, err := s.CreateRole(admin, org, RoleCreateInput{Name: "目标角色"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	// 先加载操作者快照，再让管理请求等待同一把数据库锁；
	// 等待期间提交撤权，验证请求拿到锁后会重新校验最新权限。
	if _, err = s.Authorize(actor, "iam.roles.manage", org); err != nil {
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
		finished <- s.ReplaceGrants(actor, org, target, GrantsInput{Version: 1, Permissions: []string{"iam.audit.read"}}, uuid.NewString())
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

	if err = s.ReplaceOrganizationUserRoles(admin, org, member, OrganizationUserRolesInput{RoleIDs: []string{}, AuthzEpoch: authzVersion(t, s, org)}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	for _, action := range []string{"iam.roles.manage", "iam.users.read"} {
		if _, err = s.Authorize(actor, action, org); !errors.Is(err, authorization.ErrDenied) {
			t.Fatal("empty bindings retained a fallback grant")
		}
	}
}

// TestDeleteRoleRejectsWhenUsersBound 固定「角色仍有绑定用户时不可删除」。
//
// 数据库外键 23503 也会兜底成 409，因此仅断言状态码无法区分是哪一层拒绝的。
// 这里直接调用 service，断言 ErrConflict，锁死应用层的前置校验。
func TestDeleteRoleRejectsWhenUsersBound(t *testing.T) {
	s, admin, org := domainFixture(t)

	organizationUser, err := s.CreateOrganizationUser(admin, org, OrganizationUserInput{
		Username: "bounduser", DisplayName: "绑定用户", Password: "temporary bound password",
	}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	role, err := s.CreateRole(admin, org, RoleCreateInput{
		Name: "绑定角色", Permissions: []string{"iam.audit.read"},
	}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	if err := s.ReplaceOrganizationUserRoles(admin, org, organizationUser, OrganizationUserRolesInput{
		RoleIDs: []string{role}, AuthzEpoch: authzVersion(t, s, org),
	}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	// 版本直接取自库中角色行：列表投影与 RoleGrants 的取值口径可能不同，
	// 只有拿到 DeleteRole 内部读到的同一个版本，才能证明拦截来自「仍有绑定用户」。
	var current int64
	if err := s.Store.DB.Raw(`SELECT version FROM iam.roles WHERE org_id=$1 AND id=$2`, org, role).Row().Scan(&current); err != nil {
		t.Fatal(err)
	}

	if current == 0 {
		t.Fatal("created role missing from role table")
	}

	if err := s.DeleteRole(admin, org, role, current, uuid.NewString()); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("delete role with bound users = %v, want ErrConflict", err)
	}

	// 解除绑定后同样的版本应能删除，证明上面的拒绝确实来自用户绑定。
	if err := s.ReplaceOrganizationUserRoles(admin, org, organizationUser, OrganizationUserRolesInput{
		RoleIDs: []string{}, AuthzEpoch: authzVersion(t, s, org),
	}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	if err := s.Store.DB.Raw(`SELECT version FROM iam.roles WHERE org_id=$1 AND id=$2`, org, role).Row().Scan(&current); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteRole(admin, org, role, current, uuid.NewString()); err != nil {
		t.Fatalf("delete unbound role = %v, want nil", err)
	}
}

func TestPermissionPreviewTracksEffectiveBindings(t *testing.T) {
	s, admin, org := domainFixture(t)
	member, err := s.CreateOrganizationUser(admin, org, OrganizationUserInput{Username: "previewuser", DisplayName: "Preview member", Password: "temporary preview password"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	role, err := s.CreateRole(admin, org, RoleCreateInput{Name: "Preview reader"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	if err = s.ReplaceGrants(admin, org, role, GrantsInput{Version: 1, Permissions: []string{"iam.audit.read"}}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	roles := []string{role}
	input := PreviewInput{OrganizationUserID: member, RoleIDs: &roles, AuthzEpoch: authzVersion(t, s, org)}
	preview, err := s.Preview(admin, org, input)
	if err != nil || len(preview.Added) != 1 || preview.Added[0] != "iam.audit.read" {
		t.Fatalf("new binding preview: %#v %v", preview, err)
	}

	if err = s.ReplaceOrganizationUserRoles(admin, org, member, OrganizationUserRolesInput{RoleIDs: roles, AuthzEpoch: input.AuthzEpoch}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	if _, err = s.Store.Pool.Exec(context.Background(), `UPDATE iam.permissions SET status='deprecated' WHERE permission_key='iam.audit.read'`); err != nil {
		t.Fatal(err)
	}

	input.AuthzEpoch = authzVersion(t, s, org)
	if _, err = s.Preview(admin, org, input); err != nil {
		t.Fatal("existing deprecated binding cannot be previewed", err)
	}

	if err = s.UpdateOrganizationUser(admin, org, member, OrganizationUserUpdate{Status: "disabled", AuthzEpoch: input.AuthzEpoch}, false, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	input.AuthzEpoch = authzVersion(t, s, org)
	preview, err = s.Preview(admin, org, input)
	if err != nil || len(preview.After) != 0 {
		t.Fatalf("disabled member has effective grants: %#v %v", preview, err)
	}

	other, err := s.CreateOrganizationUser(admin, org, OrganizationUserInput{Username: "previewother", DisplayName: "Other member", Password: "temporary preview password"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	input.OrganizationUserID = other
	input.AuthzEpoch = authzVersion(t, s, org)
	if _, err = s.Preview(admin, org, input); !errors.Is(err, authorization.ErrDenied) {
		t.Fatalf("new deprecated binding preview allowed: %v", err)
	}
}

func TestConfiguredAdministratorRejectsApplicationAndOperatorWrites(t *testing.T) {
	for _, operation := range []string{"profile", "password", "reset", "operator"} {
		t.Run(operation, func(t *testing.T) {
			s, actor, _ := domainFixture(t)
			user, err := repository.NewQueries(s.Store.DB).GetLoginUser(context.Background(), "admin")
			if err != nil {
				t.Fatal(err)
			}
			switch operation {
			case "profile":
				err = s.UpdateUser(actor, user.ID.String(), UserUpdate{DisplayName: "Changed", Status: "active", AuthEpoch: user.AuthEpoch}, uuid.NewString())
			case "password":
				err = s.ChangePassword(actor, "isolated domain password", "not from configuration", uuid.NewString())
			case "reset":
				err = s.ResetPassword(actor, user.ID.String(), "not from configuration", uuid.NewString())
			case "operator":
				err = s.Store.RecoverPassword(context.Background(), user.ID, "not from configuration")
			}
			if !errors.Is(err, authorization.ErrDenied) {
				t.Fatalf("configured administrator write %s was not denied: %v", operation, err)
			}
		})
	}
}

// rootOrganizationID 读取迁移创建的根组织，供需要组织上下文的用例使用。
func rootOrganizationID(t *testing.T, s *Service) string {
	t.Helper()
	var id string
	if err := s.Store.Pool.QueryRow(context.Background(), `SELECT id::text FROM iam.organizations WHERE parent_id IS NULL`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
