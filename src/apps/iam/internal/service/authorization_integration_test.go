//go:build integration

package service

import (
	"context"
	"errors"
	"sync"
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
	if err = store.Pool.QueryRow(ctx, `SELECT id::text FROM iam.organizations WHERE slug='default'`).Scan(&org); err != nil {
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
	if err := s.Store.Pool.QueryRow(context.Background(), `SELECT authz_version FROM iam.organizations WHERE id=$1`, org).Scan(&v); err != nil {
		t.Fatal(err)
	}

	return v
}

func TestCustomRoleUnionEmptyGrantsAndImmediateDelegationRevocation(t *testing.T) {
	s, admin, org := domainFixture(t)
	ctx := context.Background()
	member, err := s.CreateMember(admin, org, MemberInput{Username: "delegated", DisplayName: "委派管理员", Password: "temporary delegated password"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	actor := actorContext(t, s, "delegated")
	if _, err = s.Authorize(actor, "iam.audit.read", org); !errors.Is(err, authorization.ErrDenied) {
		t.Fatal("default member unexpectedly has business grants")
	}

	managerRole, err := s.CreateRole(admin, org, RoleCreateInput{Code: "role_manager", Name: "角色管理"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	grants := []string{"iam.roles.read", "iam.roles.manage", "iam.roles.assign", "iam.audit.read"}
	if err = s.ReplaceGrants(admin, org, managerRole, GrantsInput{Version: 1, Permissions: grants}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	readRole, err := s.CreateRole(admin, org, RoleCreateInput{Code: "member_reader", Name: "成员读取"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	if err = s.ReplaceGrants(admin, org, readRole, GrantsInput{Version: 1, Permissions: []string{"iam.members.read"}}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	if err = s.ReplaceMemberRoles(admin, org, member, MemberRolesInput{RoleIDs: []string{managerRole, readRole}, AuthzVersion: authzVersion(t, s, org)}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	for _, action := range []string{"iam.audit.read", "iam.members.read"} {
		if _, err = s.Authorize(actor, action, org); err != nil {
			t.Fatal(err)
		}
	}

	target, err := s.CreateRole(admin, org, RoleCreateInput{Code: "target_role", Name: "目标角色"}, uuid.NewString())
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

	if err = s.ReplaceMemberRoles(admin, org, member, MemberRolesInput{RoleIDs: []string{}, AuthzVersion: authzVersion(t, s, org)}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	for _, action := range []string{"iam.roles.manage", "iam.members.read"} {
		if _, err = s.Authorize(actor, action, org); !errors.Is(err, authorization.ErrDenied) {
			t.Fatal("empty bindings retained a fallback grant")
		}
	}
}

func TestConcurrentDisablesRetainAnEffectiveOrganizationAdministrator(t *testing.T) {
	s, first, _ := domainFixture(t)
	ctx := context.Background()
	firstID, err := s.CreateUser(first, UserInput{Username: "firstadmin", DisplayName: "第一管理员", Password: "regular first password"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	org, err := s.CreateOrganization(first, OrganizationInput{Slug: "concurrent-admins", Name: "并发管理员", AdminUserID: firstID}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	firstUser, err := repository.NewQueries(s.Store.DB).GetLoginUser(ctx, "firstadmin")
	if err != nil {
		t.Fatal(err)
	}

	member, err := s.CreateMember(first, org, MemberInput{Username: "secondadmin", DisplayName: "第二管理员", Password: "temporary second password"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	var adminRole string
	if err = s.Store.Pool.QueryRow(ctx, `SELECT id::text FROM iam.roles WHERE org_id=$1 AND code='org_admin'`, org).Scan(&adminRole); err != nil {
		t.Fatal(err)
	}

	if err = s.ReplaceMemberRoles(first, org, member, MemberRolesInput{RoleIDs: []string{adminRole}, AuthzVersion: authzVersion(t, s, org)}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	second := actorContext(t, s, "admin")
	secondUser, err := repository.NewQueries(s.Store.DB).GetLoginUser(ctx, "secondadmin")
	if err != nil {
		t.Fatal(err)
	}

	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, actor := range []struct {
		ctx  context.Context
		user repository.LoginUser
	}{{first, firstUser}, {second, secondUser}} {
		wg.Add(1)
		go func() {
			defer wg.Done()

			results <- s.UpdateUser(actor.ctx, actor.user.ID.String(), UserUpdate{DisplayName: actor.user.DisplayName, Status: "disabled", AuthVersion: actor.user.AuthVersion}, uuid.NewString())
		}()
	}

	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, repository.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}

	if successes != 1 || conflicts != 1 {
		t.Fatal("concurrent disables bypassed last administrator protection")
	}
}

func TestPermissionPreviewTracksEffectiveBindings(t *testing.T) {
	s, admin, org := domainFixture(t)
	member, err := s.CreateMember(admin, org, MemberInput{Username: "previewuser", DisplayName: "Preview member", Password: "temporary preview password"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	role, err := s.CreateRole(admin, org, RoleCreateInput{Code: "preview_reader", Name: "Preview reader"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	if err = s.ReplaceGrants(admin, org, role, GrantsInput{Version: 1, Permissions: []string{"iam.audit.read"}}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	roles := []string{role}
	input := PreviewInput{MembershipID: member, RoleIDs: &roles, AuthzVersion: authzVersion(t, s, org)}
	preview, err := s.Preview(admin, org, input)
	if err != nil || len(preview.Added) != 1 || preview.Added[0] != "iam.audit.read" {
		t.Fatalf("new binding preview: %#v %v", preview, err)
	}

	if err = s.ReplaceMemberRoles(admin, org, member, MemberRolesInput{RoleIDs: roles, AuthzVersion: input.AuthzVersion}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	if _, err = s.Store.Pool.Exec(context.Background(), `UPDATE iam.permissions SET status='deprecated' WHERE key='iam.audit.read'`); err != nil {
		t.Fatal(err)
	}

	input.AuthzVersion = authzVersion(t, s, org)
	if _, err = s.Preview(admin, org, input); err != nil {
		t.Fatal("existing deprecated binding cannot be previewed", err)
	}

	if err = s.UpdateMember(admin, org, member, MemberUpdate{Status: "disabled", AuthzVersion: input.AuthzVersion}, false, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	input.AuthzVersion = authzVersion(t, s, org)
	preview, err = s.Preview(admin, org, input)
	if err != nil || len(preview.After) != 0 {
		t.Fatalf("disabled member has effective grants: %#v %v", preview, err)
	}

	other, err := s.CreateMember(admin, org, MemberInput{Username: "previewother", DisplayName: "Other member", Password: "temporary preview password"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	input.MembershipID = other
	input.AuthzVersion = authzVersion(t, s, org)
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
				err = s.UpdateUser(actor, user.ID.String(), UserUpdate{DisplayName: "Changed", Status: "active", AuthVersion: user.AuthVersion}, uuid.NewString())
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
